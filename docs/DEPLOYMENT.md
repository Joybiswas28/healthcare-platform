# Deployment

One Linux server runs everything with Docker: the API, PostgreSQL, and Caddy, which serves HTTPS with free Let's Encrypt certificates.

## 1. What you need

- **A server:** Ubuntu 24.04, 2 vCPU, 4 GB RAM and 40 GB disk is enough for a pilot. For low latency in Bangladesh, choose a data centre in Singapore or Mumbai (for example DigitalOcean, Vultr, AWS Lightsail) or a local Bangladeshi provider.
- **A domain:** for example `api.yourdomain.com`. Its DNS **A record** must point to the server's IP.
- **Accounts:** an SMS provider (required for registration) and SSLCommerz (required for payments).

## 2. Prepare the server (once)

```sh
# as root on the server
apt update && apt upgrade -y
curl -fsSL https://get.docker.com | sh
ufw allow OpenSSH && ufw allow 80 && ufw allow 443 && ufw enable   # only SSH and web are open
adduser deploy && usermod -aG docker deploy
```

Log in with an SSH key and turn off password login (`PasswordAuthentication no` in `/etc/ssh/sshd_config`).

## 3. Install

```sh
sudo -iu deploy
git clone https://github.com/Joybiswas28/healthcare-platform.git /opt/healthcare   # private repo: use a deploy key
cd /opt/healthcare/deploy
cp .env.example .env
nano .env          # fill in every value; generate secrets with: openssl rand -hex 32
chmod 600 .env
docker compose -f docker-compose.prod.yml up -d --build
```

Check that it works:

```sh
curl https://api.yourdomain.com/readyz        # {"status":"ready"}
docker compose -f docker-compose.prod.yml logs -f api
```

## 4. Create the first admin

```sh
docker compose -f docker-compose.prod.yml exec api /app/createadmin \
  -name "Your Name" -phone 01XXXXXXXXX -password "a-long-unique-password"
```

## 5. Backups

```sh
chmod +x /opt/healthcare/deploy/backup.sh
crontab -e
# add:
15 3 * * * /opt/healthcare/deploy/backup.sh >> /home/deploy/backup.log 2>&1
```

Copy `/var/backups/healthcare` to another place every day, such as another server or cloud storage. **Test a restore once** before you rely on it. The commands are at the bottom of `backup.sh`.

## 6. Updating

```sh
cd /opt/healthcare && git pull
cd deploy && docker compose -f docker-compose.prod.yml up -d --build api
```

Database migrations run automatically when the API starts. There is a few seconds of downtime while the container restarts.

## 7. Turning on the external services

| Service | Settings in `deploy/.env` | Then |
|---|---|---|
| SSLCommerz | `SSLCOMMERZ_STORE_ID`, `SSLCOMMERZ_STORE_PASSWORD`, `SSLCOMMERZ_SANDBOX` (`true` for testing, `false` for live) | Restart the API. Test a purchase in sandbox first |
| SMS | Needs a small integration for the chosen provider | Not yet built: each provider has its own API |
| Video calls (TURN) | `TURN_URLS` (e.g. `turn:turn.example.com:3478,turns:turn.example.com:5349`), `TURN_USERNAME`, `TURN_CREDENTIAL` | Run a TURN server such as coturn. Without it, calls between some mobile networks fail to connect. Restart the API |

## Checklist before real patients

- [ ] HTTPS works and `http://` redirects (Caddy does this automatically)
- [ ] `deploy/.env` has random secrets and `chmod 600`
- [ ] SMS provider connected: register a test account with a real OTP
- [ ] SSLCommerz sandbox purchase works end to end, then switch to live
- [ ] Daily backups run, and a restore was tested
- [ ] First admin created; test doctor, hospital and pharmacy verified
- [ ] Privacy policy, terms and refund policy published on the website (also required by SSLCommerz)
