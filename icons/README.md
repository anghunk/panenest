# Icons

- `f7-*.svg`: [Framework7 Icons](https://framework7.io/icons/) (MIT, © Vladimir Kharlampidi), drawn after Apple's SF Symbols.
- `meteor-*.svg`: [Meteor Icons](https://github.com/zkreations/icons) (MIT, © zkreations).
- `si-*.svg`: [Simple Icons](https://simpleicons.org) (CC0), programs' logos; the brands are their owners' trademarks.
- `octicon-*.svg`: [Octicons](https://primer.style/octicons) (MIT, © GitHub), for what Framework7 has no icon of, as a file's diff.
- `lucide-*.svg`: [Lucide](https://lucide.dev) (ISC, © Lucide Contributors), with a heavier stroke than Iconify serves (`stroke-width`), as SF Symbols' semibold: 2.5 for `chevrons-up-down`, 3 for `check`.

All as [Iconify](https://iconify.design) serves them: `https://api.iconify.design/f7/<name>.svg`.

`resources/icon.png` 和 `resources/icon-dev.png` 使用用户提供的 `applore-icon-4653.png`（1024×1024 终端窗口与螃蟹图标），分别用于正式版和开发版，由 MyGo 打包为各平台应用图标。

`tray-template.png` 是螃蟹的黑色透明底轮廓，保留眼睛高光镂空，供 macOS 在菜单栏中自动着色：轮廓最长边为 14 点，居中放在 18×18 点画布中，按 2 倍分辨率提供。`tray.png` 是 64×64 的彩色应用图标，用于 Windows 和 Linux 的通知区域。
