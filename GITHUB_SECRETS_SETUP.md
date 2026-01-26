# GitHub Secrets Setup Guide

## Description
This guide helps you add the required GitHub Secrets for automated deployment.

## Prerequisites
- Admin access to your GitHub repository
- Access to your current `.env` file on the droplet (or the values from `chatbasket-api/.env`)

## Steps

### 1. Navigate to GitHub Secrets

1. Go to your repository: `https://github.com/wpbasket/chatbasket_backend`
2. Click **Settings** → **Secrets and variables** → **Actions**
3. Click **New repository secret**

### 2. Add Each Secret

You need to add **23 secrets**. For each one:

1. Click **New repository secret**
2. Enter the **Name** (exactly as shown below)
3. Paste the **Value** from your current `.env` file
4. Click **Add secret**

### Required Secrets

| Secret Name | Source | 
|-------------|--------|
| `APPWRITE_ENDPOINT` | Line 1 of .env |
| `APPWRITE_PROJECT_ID` | Line 2 of .env |
| `APPWRITE_API_KEY` | Line 3 of .env |
| `APPWRITE_DATABASE_ID` | Line 4 of .env |
| `APPWRITE_USERS_COLLECTION_ID` | Line 5 of .env |
| `APPWRITE_POSTS_COLLECTION_ID` | Line 6 of .env |
| `APPWRITE_REFRESH_TOKENS_COLLECTION_ID` | Line 7 of .env |
| `APPWRITE_LIKES_COLLECTION_ID` | Line 8 of .env |
| `APPWRITE_FOLLOW_COLLECTION_ID` | Line 9 of .env |
| `APPWRITE_COMMENTS_COLLECTION_ID` | Line 10 of .env |
| `APPWRITE_BLOCK_COLLECTION_ID` | Line 11 of .env |
| `APPWRITE_FOLLOW_REQUESTS_COLLECTION_ID` | Line 12 of .env |
| `APPWRITE_TEMP_OTP_COLLECTION_ID` | Line 13 of .env |
| `APPWRITE_FILE_USERPROFILEPIC_BUCKET_ID` | Line 14 of .env |
| `APPWRITE_PERSONAL_DATABASE_ID` | Line 15 of .env |
| `APPWRITE_PERSONAL_USERS_COLLECTION_ID` | Line 16 of .env |
| `APPWRITE_PERSONAL_ALONE_USERNAME_COLLECTION_ID` | Line 17 of .env |
| `PERSONAL_USERNAME_KEY` | Line 18 of .env |
| `DATABASE_URL_PG_CB` | Line 19 of .env |
| `DATABASE_URL_PG_DEV` | Line 20 of .env |
| `APPWRITE_FILE_PERSONAL_USERPROFILEPIC_BUCKET_ID` | Line 21 of .env |
| `COSMOS_CONNECTION_STRING` | Line 29 of .env |
| `COSMOS_DATABASE` | Line 30 of .env |
| `COSMOS_CONTAINER` | Line 31 of .env |

### 3. Verify Existing Secrets

Make sure these already exist (should be configured from previous setup):
- `SSH_USER`
- `SSH_PRIVATE_KEY`
- `DROPLET_IP`

### 4. Quick Copy Helper

If you have SSH access to the droplet, you can copy values:

```bash
# SSH into droplet
ssh root@your-droplet-ip

# View .env file
cd /app/chatbasket
cat .env
```

Then copy each value into GitHub Secrets.

## What Happens After Setup

Once all secrets are added:
1. Push any change to `main` branch
2. GitHub Actions will automatically:
   - Build Docker image
   - Push to GHCR
   - Copy `docker-compose.yml` and `nginx.conf` to droplet
   - Generate `.env` from GitHub Secrets
   - Restart containers

## Verification

After pushing a commit:
1. Go to **Actions** tab in GitHub
2. Watch the workflow run
3. Verify it completes without errors
4. SSH to droplet and check files were updated:
   ```bash
   ls -lh /app/chatbasket/
   cat /app/chatbasket/.env | head -5
   docker ps
   ```

## Security Notes

✅ Secrets are encrypted by GitHub  
✅ Never appear in logs  
✅ Only accessible to workflows  
❌ Never commit `.env` to Git  
❌ Never share secret values publicly
