services:
  api:
    image: ghcr.io/wpbasket/chatbasket-api:latest
    container_name: chatbasket-api
    restart: always
    env_file:
      - .env
    expose:
      - "8080"

  nginx:
    image: nginx:alpine
    container_name: chatbasket-nginx
    restart: always
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - /etc/ssl/cloudflare:/etc/ssl/cloudflare:ro
    depends_on:
      - api

