#!/usr/bin/env bash
# Loads optional local .env / .env.local Podcast Index credentials without
# overwriting explicit environment variables and prints Go -ldflags flags for
# build-time embedding in local desktop and web builds.

import_local_dotenv() {
    local project_root="$1"
    local env_file line name val existing
    for env_file in ".env" ".env.local"; do
        local full_path="${project_root}/${env_file}"
        [[ -f "$full_path" ]] || continue
        while IFS= read -r line || [[ -n "$line" ]]; do
            line="${line%$'\r'}"
            [[ -z "$line" || "$line" == \#* || "$line" != *"="* ]] && continue
            name="${line%%=*}"
            val="${line#*=}"
            name="${name#"${name%%[![:space:]]*}"}"
            name="${name%"${name##*[![:space:]]}"}"
            val="${val#"${val%%[![:space:]]*}"}"
            val="${val%"${val##*[![:space:]]}"}"
            if [[ ${#val} -ge 2 ]]; then
                if [[ ("$val" == \"*\" && "$val" == *\") || ("$val" == \'*\' && "$val" == *\') ]]; then
                    val="${val:1:${#val}-2}"
                fi
            fi
            [[ -z "$name" ]] && continue
            existing="${!name:-}"
            if [[ -z "$existing" && -n "$val" ]]; then
                export "${name}=${val}"
            fi
        done < "$full_path"
    done
}

podcast_index_ldflags() {
    local project_root="$1"
    import_local_dotenv "$project_root"

    local api_key="${VIIB_PODCAST_INDEX_API_KEY:-${PODCAST_INDEX_API_KEY:-}}"
    local api_secret="${VIIB_PODCAST_INDEX_API_SECRET:-${PODCAST_INDEX_API_SECRET:-}}"
    api_key="${api_key//\'/}"
    api_secret="${api_secret//\'/}"

    if [[ -z "$api_key" || -z "$api_secret" ]]; then
        return 0
    fi

    local pkg="github.com/ajbergh/viib-mediahub/internal/podcast"
    printf -- "-X '%s.embeddedDirectoryAPIKey=%s' -X '%s.embeddedDirectoryAPISecret=%s'" \
        "$pkg" "$api_key" "$pkg" "$api_secret"
}
