# hello-node 的 multi-stage Dockerfile（第 08 章，Node 對照組）
#
#   docker build -t hello-node:ch08 tutorials/08-multi-stage/node
#   docker run --rm -p 8080:8080 hello-node:ch08
#
# 注意 build context 是 node/ 這個資料夾本身（不是 app/）。

# ── build stage：完整安裝（含 devDependencies），需要的話在這裡 build ──
FROM node:24-slim AS build
WORKDIR /app
# 先只複製 package.json / package-lock.json：依賴沒變，npm ci 這層就吃快取（第 07 章）
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
# 有 TypeScript / bundler 的話在這裡跑：RUN npm run build
# 砍掉 devDependencies，只留上線需要的
RUN npm prune --omit=dev

# ── runtime stage：還是需要 node 本體，但不需要 npm cache、devDependencies ──
FROM node:24-slim AS runtime
ENV NODE_ENV=production
WORKDIR /app
COPY --from=build --chown=node:node /app/node_modules ./node_modules
COPY --chown=node:node package.json server.js ./
# node 官方 image 內建 uid 1000 的 node 使用者，不要用 root 跑（第 21 章細講）
USER node
EXPOSE 8080
CMD ["node", "server.js"]
