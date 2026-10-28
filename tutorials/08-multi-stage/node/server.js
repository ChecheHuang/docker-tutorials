// hello-node —— 第 08、09、23 章用來跟 shorty（Go）對照的最小 Node 服務。
//
//   npm install
//   npm start
//   curl localhost:8080/

const express = require("express");

const app = express();
const port = Number(process.env.PORT) || 8080;

app.get("/", (_req, res) => {
  res.type("text/plain").send("hello from Node (express)\n");
});

// 要聽 0.0.0.0：container 裡聽 127.0.0.1 的話，-p 轉進來的連線會連不到（第 12 章）
const server = app.listen(port, "0.0.0.0", () => {
  console.log(`listening on 0.0.0.0:${port}`);
});

// docker stop 送 SIGTERM；node 當 PID 1 時預設不會處理，要自己接（第 16 章）
for (const sig of ["SIGTERM", "SIGINT"]) {
  process.on(sig, () => server.close(() => process.exit(0)));
}
