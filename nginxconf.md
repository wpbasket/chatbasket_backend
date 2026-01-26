worker_processes auto;

events {}

http {
    # -------------------------
    # HTTP → HTTPS redirect
    # -------------------------
    server {
        listen 80;
        server_name chatbasket.live *.chatbasket.live;

        return 301 https://$host$request_uri;
    }

    # -------------------------
    # HTTPS server (Cloudflare)
    # -------------------------
    server {
        listen 443 ssl http2;
        server_name chatbasket.live *.chatbasket.live;

        ssl_certificate     /etc/ssl/cloudflare/origin.pem;
        ssl_certificate_key /etc/ssl/cloudflare/origin.key;

        ssl_protocols TLSv1.2 TLSv1.3;

        location / {
            proxy_pass http://api:8080;
            proxy_http_version 1.1;

            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto https;
        }
    }
}

