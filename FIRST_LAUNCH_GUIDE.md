# First Launch Configuration Guide

## Overview
ViiB MediaHub now includes a comprehensive first-launch configuration wizard that guides new users through the initial setup process. This feature provides a welcoming onboarding experience with music folder selection and optional Spotify integration.

## Features

### Multi-Step Wizard
The first launch dialog consists of 4 steps:

1. **Welcome Screen** - Overview of features and capabilities
2. **Add Music Folders** - Select folders containing music files to scan
3. **Spotify Integration** (Optional) - Configure Spotify API credentials for metadata enrichment
4. **Complete Setup** - Review and start initial library scan

### Key Capabilities

- **Folder Browser**: Navigate the file system to select music directories
- **Multiple Folders**: Add multiple scan folders during setup
- **Skip Options**: Users can skip steps or defer setup entirely
- **Spotify Setup**: Optional configuration for:
  - Album artwork enrichment
  - Artist metadata and images
  - Spotify Premium downloads
- **Progress Tracking**: Visual progress bar showing setup completion
- **Persistent State**: Setup completion is saved to localStorage

## User Flow

### Step 1: Welcome
- Displays ViiB MediaHub branding and overview
- Shows three key feature cards:
  - Add Music
  - Spotify Sync
  - Start Listening
- Options:
  - "Let's Get Started" → Continue to Step 2
  - "Skip setup for now" → Close dialog and start using app

### Step 2: Add Music Folders
- Shows list of currently added folders (initially empty)
- "Add Folder" button opens folder browser
- Folder browser allows navigation through directory structure
- Users can add multiple folders
- Selected folders display with path and remove option
- Options:
  - "Back" → Return to Welcome
  - "Skip" → Jump to completion
  - "Continue" → Proceed to Spotify setup (disabled if no folders)

### Step 3: Spotify Integration (Optional)
- Explains benefits of Spotify integration:
  - High-resolution album artwork
  - Rich metadata (genres, release dates, descriptions)
  - Spotify Premium downloads
- Input fields for:
  - Spotify Client ID
  - Spotify Client Secret
- Link to Spotify Developer Dashboard for obtaining credentials
- Options:
  - "Back" → Return to folder setup
  - "Skip for now" → Jump to completion
  - "Save & Continue" → Save credentials and proceed (disabled if fields empty)

### Step 4: Complete Setup
- Shows success message with checkmark
- Displays "What's Next?" section with guidance:
  - Scan Your Music (if folders added)
  - Explore Features
  - Enjoy Your Music
- Options based on setup:
  - **With folders**: 
    - "I'll Scan Later" → Close dialog
    - "Start Scanning" → Begin library scan with progress updates
  - **Without folders**:
    - "Start Using ViiB MediaHub" → Close dialog

## Technical Implementation

### State Management
- Uses Zustand store with `hasCompletedSetup` flag
- Flag persisted to localStorage via `mediahub-storage`
- Dialog shown when: `backendAvailable && !hasCompletedSetup`
- **Intelligent Detection**: Automatically checks database for existing configuration on startup:
  - Queries scan folders, songs, and Spotify credentials
  - If any configuration data exists, `hasCompletedSetup` is automatically set to `true`
  - Prevents dialog from appearing on existing installations or after database restore

### Components
- **FirstLaunchDialog.tsx**: Main wizard component
- **Folder Browser**: Reusable modal for directory navigation
- **Progress Indicator**: Visual step tracker (Steps 2-3)

### Integration Points
- **App.tsx**: Renders dialog at root level
- **UISlice**: Manages `hasCompletedSetup` state
- **API**: Uses existing folder management and Spotify credential endpoints

## Testing

### Reset First Launch State
To test the first launch dialog again, clear the setup flag from localStorage:

```javascript
// In browser console:
const storage = JSON.parse(localStorage.getItem('mediahub-storage'));
storage.state.hasCompletedSetup = false;
localStorage.setItem('mediahub-storage', JSON.stringify(storage));
location.reload();
```

Or completely clear localStorage:
```javascript
localStorage.clear();
location.reload();
```

### Test Scenarios

1. **Complete Setup Path**:
   - Step through all 4 steps
   - Add at least one folder
   - Enter Spotify credentials
   - Start library scan

2. **Minimal Setup**:
   - Add folders only
   - Skip Spotify integration
   - Complete without scanning

3. **Skip Everything**:
   - Click "Skip setup for now" on welcome screen
   - Verify dialog doesn't reappear

4. **Browser Mode (No Backend)**:
   - Dialog should not appear if `backendAvailable === false`
   - Users use legacy file picker in Settings

5. **Existing Configuration Detection**:
   - Clear localStorage but keep SQLite database with existing data
   - Launch app - dialog should NOT appear
   - App detects existing scan folders/songs/credentials automatically
   - Setup is marked complete without user interaction

## Design Philosophy

The first launch dialog follows ViiB MediaHub's design system:

- **Colors**: Brand green (`#1db954`), surface grays, text hierarchy
- **Typography**: Inter font family, clear hierarchy
- **Spacing**: Consistent padding and gaps
- **Animations**: Smooth fade-ins and transitions
- **Icons**: Lucide icons matching existing UI
- **Modals**: Backdrop blur, centered layout, escape to close
- **Buttons**: Primary (brand green), secondary (surface), text links

## Future Enhancements

Potential improvements for future versions:

- [ ] Detect common music folder locations automatically
- [ ] Show estimated scan time based on folder size
- [ ] Allow folder removal during scan selection
- [ ] Add iTunes library import option
- [ ] Include audio format detection (MP3, FLAC, OGG, etc.)
- [ ] Progressive disclosure for advanced settings
- [ ] Video tutorial or help tooltips
- [ ] Dark/light theme selection during setup
- [ ] Language selection for internationalization
