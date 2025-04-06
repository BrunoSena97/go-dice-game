module.exports = {
  apps: [
    {
      name: "dice-game-frontend",
      script: "./build/index.js",
      env: {
        PORT: "4300",
        HOST: "0.0.0.0",
      },
    },
  ],
};
