# Icons

- `f7-*.svg`: [Framework7 Icons](https://framework7.io/icons/) (MIT, © Vladimir Kharlampidi), drawn after Apple's SF Symbols.
- `meteor-*.svg`: [Meteor Icons](https://github.com/zkreations/icons) (MIT, © zkreations).
- `si-*.svg`: [Simple Icons](https://simpleicons.org) (CC0), programs' logos; the brands are their owners' trademarks.
- `octicon-*.svg`: [Octicons](https://primer.style/octicons) (MIT, © GitHub), for what Framework7 has no icon of, as a file's diff.
- `lucide-*.svg`: [Lucide](https://lucide.dev) (ISC, © Lucide Contributors), with a heavier stroke than Iconify serves (`stroke-width`), as SF Symbols' semibold: 2.5 for `chevrons-up-down`, 3 for `check`.

All as [Iconify](https://iconify.design) serves them: `https://api.iconify.design/f7/<name>.svg`.

`resources/icon.png` 和 `resources/icon-dev.png` 是 1024×1024 的透明底单只螃蟹，分别用于正式版和开发版，由 MyGo 打包为 Windows 和 Linux 应用图标。Windows 可执行文件和安装包使用 MyGo 生成的多尺寸图标，并保留透明通道。`resources/icon-macos.icns` 保留原有终端窗口与螃蟹图案，通过 `mygo.json` 的 `macos.icon` 供 macOS 使用。

`tray-template.png` 是螃蟹的黑色透明底轮廓，保留眼睛高光镂空，供 macOS 在菜单栏中自动着色：轮廓最长边为 14 点，居中放在 18×18 点画布中，按 2 倍分辨率提供。`tray.png` 是从上述透明螃蟹缩小的 64×64 彩色图标，用于 Windows 和 Linux 的通知区域。

透明螃蟹通过内置 imagegen 从原有图案提取，提示词要求：保留单只橙红渐变螃蟹、双螯、六条腿和眼睛高光；移除终端窗口、按钮、提示符和黑底；居中、完整显示，使用真实 alpha 透明背景，不添加其他对象。
