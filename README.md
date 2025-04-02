# Rest API to send mail in Mail in a Box server

You have a Mail in a Box server. It only supports sending mail directly using port. What if you need a REST API to send
mail? This script will handle this.

It will not install any new packages to packages to server. Just run the script again after a Mail in a Box's version
upgrade to
retrain
the feature.

And if you want to remove it completely, there's a script also.

# Steps

## First compile the main.go as mail-api

```bash
go build -o mail-api main.go
```

## Then generate SSL for you domain or subdomain using

```shell
sudo certbot certonly --manual --preferred-challenges dns -d domain.com
```

## Follow the instructions provided by Certbot:

- It will ask you to create a DNS TXT record to verify domain ownership
- You'll need to add this record through your DNS provider
- After adding the record, wait a moment for DNS propagation, then continue

Once verification is complete, Certbot will save the certificates to /etc/letsencrypt/live/domain.com/:

- fullchain.pem (the certificate plus the complete certificate chain)
- privkey.pem (the private key)

## Update the script

```shell
SERVICE_NAME="mail-api" 
GO_BINARY_PATH="/usr/local/bin/mail-api"
SERVICE_PORT=1111 # update this port if you need
WORKING_DIR="/home"
DOMAIN_NAME=domain.com # your domain
GO_API_PORT=1112 # go api running port
```

## Running the script

First download the executable & .sh files to the server using scp

```shell
scp mail_api root@ip:/home 
scp setup.sh root@ip:/home 
scp remove.sh root@ip:/home 
chmod +x setup.sh # Make the setup script executable
chmod +x remove.sh
```

Then run

```shell
sudo ./setup.sh
```

And if everything goes well, check the API using

```shell
curl -X POST \
  -H "Authorization: Basic $(echo -n 'noreply@mail.com:password' | base64)" \
  -H "Content-Type: application/json" \
  -d '{
    "to": ["recipient@example.com"],
    "subject": "Test Email",
    "content": "This is a test email.",
    "title": "Title"
  }' \
  https://domain.com:1111/mail/send
```

Anf if you want to remove all this just run

```shell
sudo ./remove.sh
```

This is how you can setup an REST API in your Mail in a Box server to sending mail.

# Credits

These scripts are compiled by [Shahriar Nasim Nafi](https://github.com/SNNafi)