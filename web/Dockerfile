FROM node:22-alpine AS builder
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
ENV NODE_OPTIONS=--max-old-space-size=400
RUN npm run build
FROM nginxinc/nginx-unprivileged:alpine
COPY --from=builder --chown=101:101 /src/dist /usr/share/nginx/html
COPY --chown=101:101 nginx.conf /etc/nginx/nginx.conf
USER 101:101
ENTRYPOINT ["nginx"]
CMD ["-g", "daemon off;"]
