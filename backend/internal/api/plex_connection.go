// Validates Plex routes and recovers account-managed connections after read failures.
package api

import (
	"context"
	"errors"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/plex"
)

func (a *API) newPlexAuthClient() *plex.AuthClient {
	if a.plexAuthFactory != nil {
		return a.plexAuthFactory()
	}
	return plex.NewAuthClient()
}

func validatePlexRoutes(ctx context.Context, machine, token, clientID string, routes []string) (plex.Server, error) {
	var firstErr error
	for _, addr := range routes {
		if err := ctx.Err(); err != nil {
			return plex.Server{}, err
		}
		client, err := plex.NewClient(addr, token, clientID)
		if err == nil {
			attempt, cancel := context.WithTimeout(ctx, 6*time.Second)
			server, validationErr := client.ValidateServerForMachine(attempt, machine)
			cancel()
			if validationErr == nil {
				return server, nil
			}
			err = validationErr
		}
		if firstErr == nil || isPlexAuthError(err) {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = errors.New("no usable address advertised by Plex for this server")
	}
	return plex.Server{}, firstErr
}

// retryPlexRead retries a read once after refreshing a rejected token or
// validating another route for an account-managed server.
func retryPlexRead[T any](a *API, ctx context.Context, source *db.PlexSource, read func(*plex.Client) (T, error)) (T, error) {
	client, err := a.plexClientForSource(ctx, source)
	if err != nil {
		var zero T
		return zero, err
	}
	value, err := read(client)
	if err == nil || ctx.Err() != nil || !isPlexConnectionError(err) {
		return value, err
	}
	retry := a.recoverPlexClient(ctx, source, client, isPlexAuthError(err))
	if retry == nil {
		return value, err
	}
	return read(retry)
}

func (a *API) recoverPlexClient(ctx context.Context, source *db.PlexSource, failed *plex.Client, rejectedToken bool) *plex.Client {
	if !rejectedToken && !source.AccountManaged {
		return nil
	}
	a.plexAuthMu.Lock()
	defer a.plexAuthMu.Unlock()

	current, err := a.db.GetPlexSource(source.ID)
	if ctx.Err() != nil || err != nil || current == nil || !current.Active || current.ConnectedAt != source.ConnectedAt {
		return nil
	}
	credentials, err := a.loadPlexCredentials()
	if err != nil || credentials.AccountToken == "" {
		return nil
	}
	token := credentials.ServerTokens[source.MachineIdentifier]
	if current.BaseURL != failed.BaseURL() || (rejectedToken && token != "" && !failed.UsesToken(token)) {
		client, _ := plex.NewClient(current.BaseURL, token, credentials.ClientIdentifier)
		return client
	}

	if time.Since(a.plexRecoveryAttempts[source.ID]) < 30*time.Second {
		return nil
	}
	if a.plexRecoveryAttempts == nil {
		a.plexRecoveryAttempts = make(map[string]time.Time)
	}
	a.plexRecoveryAttempts[source.ID] = time.Now()
	recovery, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	previousAccountToken, previousServerToken := credentials.AccountToken, token
	defer func() {
		if credentials.AccountToken != previousAccountToken || credentials.ServerTokens[source.MachineIdentifier] != previousServerToken {
			if err := a.savePlexCredentials(credentials); err != nil {
				logger.API("Failed to save Plex credentials after recovery: %v", err)
			}
		}
	}()
	token, routes, err := a.newPlexAuthClient().ResolveServerRoutes(recovery, &credentials, source.MachineIdentifier)
	if err != nil {
		logger.API("Plex credential or route recovery failed for source %s: %v", source.ID, plex.RedactError(err))
		return nil
	}
	addr := current.BaseURL
	if !rejectedToken {
		alternates := make([]string, 0, len(routes))
		for _, route := range routes {
			if route != current.BaseURL {
				alternates = append(alternates, route)
			}
		}
		server, err := validatePlexRoutes(recovery, source.MachineIdentifier, token, credentials.ClientIdentifier, alternates)
		if err != nil {
			return nil
		}
		if err := a.db.UpdatePlexVerifiedRoute(recovery, *current, server.URL, server.Name, server.Version); err != nil {
			logger.API("Failed to save the recovered Plex route for source %s: %v", source.ID, err)
			return nil
		}
		addr = server.URL
	} else if failed.UsesToken(token) {
		return nil
	}
	credentials.ServerTokens[source.MachineIdentifier] = token
	client, _ := plex.NewClient(addr, token, credentials.ClientIdentifier)
	return client
}
