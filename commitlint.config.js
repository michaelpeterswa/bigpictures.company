module.exports = {
  extends: ['@commitlint/config-conventional'],
  ignores: [
    (message) => message.includes('Initial commit'),
    (message) => message.includes('Initial bigpictures.company implementation'),
  ],
};
