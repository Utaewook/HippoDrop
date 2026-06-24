# The Ultimate Beginner's Guide to Google Cloud Setup for HippoDrop

Don't worry if you've never used Google Cloud before! Just follow these steps exactly, and you'll be done in 3 minutes.

---

## Step 1: Create a Project & Enable the API
1. Go to **https://console.cloud.google.com/apis/library/drive.googleapis.com**
2. If prompted, agree to the terms of service.
3. You will see "Google Drive API". Click the blue **[Enable]** button.
   *(If it asks you to create a project first, click "Create Project", name it "hippodrop-storage", and click "Create". Then click [Enable] again).*

---

## Step 2: Configure the "Consent Screen" (Crucial Step!)
Before you can get credentials, Google needs to know who is allowed to use this app.
1. On the left sidebar menu (≡), click **APIs & Services** > **OAuth consent screen**.
2. Under "User Type", select **External** and click **[Create]**.
3. **App information**: Type "HippoDrop" for the App name, and select your email from the dropdown for the support email.
4. **Developer contact information**: Type your email address again at the very bottom.
5. Click **[Save and Continue]**.
6. **Scopes**: Do nothing here. Just click **[Save and Continue]**.
7. **Test users (★ THE MOST IMPORTANT PART ★)**: 
   - Click the **[+ ADD USERS]** button.
   - Type your exact Gmail address (the one you will use to log in).
   - Click **[Add]**, then click **[Save and Continue]**.
   *(If you skip adding your email here, you will get a "403 Access Denied" error later!)*
8. **Summary**: Scroll to the bottom and click **[Back to Dashboard]**.

---

## Step 3: Get Your Client Secret JSON
Now we just need to download the actual key file.
1. On the left sidebar menu, click **APIs & Services** > **Credentials**.
2. At the top of the screen, click **[+ CREATE CREDENTIALS]** > **OAuth client ID**.
3. Under "Application type", open the dropdown and select **Desktop app**.
4. Name it anything (e.g., "HippoDrop Client") and click **[Create]**.
5. A popup will appear saying "OAuth client created". Click the **[DOWNLOAD JSON]** button.
6. Move this downloaded file to a safe folder on your computer.

---

## Step 4: Finish HippoDrop Setup
1. Press **'q'** or **'Esc'** to close this guide.
2. In the HippoDrop setup wizard, paste the **Absolute Path** to the JSON file you just downloaded.
   *(Example: `/home/user/client_secret.json` or `C:\Users\admin\client_secret.json`)*
3. Complete the wizard. HippoDrop will give you a link to open in your browser to log in securely!
