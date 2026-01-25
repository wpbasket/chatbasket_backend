version: "3.9"

services:
  api:
    image: ghcr.io/wpbasket/chatbasket-api:latest
    container_name: chatbasket-api
    restart: always
    env_file:
      - .env
    ports:
      - "8080:8080"
