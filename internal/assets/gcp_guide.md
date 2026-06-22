# Google Cloud Platform (GCP) Credential Guide

Tardis connects to Google Drive securely using OAuth 2.0 User Consent. Follow these 4 steps to create your credentials and authorize the application:

---

## Step 1. Create a GCP Project
1. Open the Google Cloud Console (https://console.cloud.google.com) and log in.
2. Click the project dropdown at the top-left corner (next to the logo).
3. Click "New Project" in the top-right of the popup window.
4. Enter a project name (e.g., "tardis-storage") and click "Create". (It takes a few seconds to initialize.)

---

## Step 2. Enable Google Drive API
1. Open the navigation menu (top-left 3 bars) > "APIs & Services" > "Library".
2. Search for "Google Drive API" and click on it.
3. Click the blue "Enable" button.

---

## Step 3. Configure OAuth Consent Screen
1. Open the navigation menu > "APIs & Services" > "OAuth consent screen".
2. Choose "External" (or "Internal" if you have a Workspace account) and click "Create".
3. Fill in the required fields (App name, User support email, Developer contact information).
4. Click "Save and Continue" through Scopes and Test Users. (If "External", add your own email to "Test Users" so you can test it without publishing the app).

---

## Step 4. Create OAuth Client ID
1. Go to "APIs & Services" > "Credentials".
2. Click "+ Create Credentials" at the top and select "OAuth client ID".
3. For "Application type", select "Desktop app".
4. Enter a name (e.g., "Tardis Client") and click "Create".
5. A popup will appear. Click "DOWNLOAD JSON" to save the client secret file.
6. Provide the absolute path to this JSON file in the Tardis wizard.

---

Press 'q' or 'Esc' to return to the setup wizard.
