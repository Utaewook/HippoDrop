# Google Cloud Platform (GCP) Credential Guide

Tardis connects to Google Drive securely using a Google Cloud Service Account. Follow these 5 steps to create your credentials and link your drive:

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

## Step 3. Create a Service Account
1. Open the navigation menu > "APIs & Services" > "Credentials".
2. Click "+ Create Credentials" at the top and select "Service Account".
3. Enter a name (e.g., "tardis-worker") and click "Create and Continue".
4. You can skip the role assignment step and click "Done".

---

## Step 4. Generate JSON Key File
1. Under "Service Accounts" list, click the email address of the account you just created.
   (Format: tardis-worker@<project-id>.iam.gserviceaccount.com)
   👉 COPY THIS EMAIL ADDRESS. You will need it in Step 5.
2. Click the "Keys" tab at the top.
3. Click "Add Key" > "Create new key".
4. Choose "JSON" as the key type and click "Create".
5. A JSON file will download to your computer. This file is your "credentials.json".

---

## Step 5. Link with Google Drive (★ CRITICAL ★)
1. Go to your regular Google Drive (https://drive.google.com).
2. Create a new folder (e.g., "tardis_data") which Tardis will use.
3. Right-click the folder and select "Share" > "Share".
4. Paste the Service Account Email Address copied in Step 4.
5. Set the permission role to "Editor" and click "Send".

---

Press 'q' or 'Esc' to return to the setup wizard.
