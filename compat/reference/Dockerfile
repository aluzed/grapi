FROM node:24.22.1-slim
ENV NODE_ENV=production STRAPI_TELEMETRY_DISABLED=true
WORKDIR /app
COPY project/package.json project/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY project/config ./config
COPY project/bootstrap ./bootstrap
COPY entrypoint.sh /entrypoint.sh
RUN mkdir -p /data public/uploads && chown -R node:node /app /data
USER node
EXPOSE 1337
ENTRYPOINT ["/entrypoint.sh"]
