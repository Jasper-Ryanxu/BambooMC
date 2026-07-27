# BambooMC

<p align="center">
基于 <code>go-mc</code> 深度二次开发的新一代 Minecraft Java 版服务端核心
<br/>
<small>A new-generation Minecraft: Java Edition server core secondarily developed based on <code>go-mc</code></small>
</p>

---

## 💡 项目是什么？ | What is BambooMC?
**中文**
BambooMC 是依托开源框架 **go-mc** 二次开发打造的 Minecraft 服务端核心。
在原生 go-mc 网络协议底层之上，大幅重写世界逻辑、生物系统、地形生成、服务端调度机制，补足原生框架服务端能力短板，打造一套功能完善、可直接用于开服的现代化 MC 服务端。

**English**
BambooMC is a Minecraft server core built through secondary development based on the open-source framework **go-mc**.
Based on the native network protocol of go-mc, we have extensively rewritten world logic, mob systems, terrain generation and server scheduling mechanisms. It makes up for the deficiencies of the original framework and forms a fully functional modern Minecraft server ready for deployment.

## ⭐ 核心优势 | Core Features
| 特性 Features | 描述 Description |
| :--- | :--- |
| 📦 单 EXE 即可运行<br>Single EXE executable | 无需 Java 环境，无复杂依赖，仅单个可执行文件直接启动服务端，开箱即用<br>No Java required, no complicated dependencies. Start the server directly with one executable file. |
| ⚡ Go 原生并发架构<br>Go native concurrency | 依托 Goroutine 协程处理网络、区块、实体、玩家事件，内存占用低，运行稳定低延迟<br>Goroutines handle network, chunk, entity and player events. Low memory usage, stable and low-latency. |
| 🧠 先进自研生物 AI<br>Advanced custom mob AI | 重构生物寻路与行为逻辑，优化大量实体运算压力，生物行为更加流畅自然<br>Rewritten mob pathfinding & behaviour logic. Optimized computation for massive entities for smoother performance. |
| 🔌 丰富扩展体系<br>Powerful extension system | 模块化架构，内置事件钩子，开放拓展接口，支持开发者开发插件与自定义玩法<br>Modular architecture with event hooks and open APIs for plugins and custom game mechanics. |
| 🔄 长期持续更新<br>Active & continuous updates | 项目持续迭代维护，不断优化底层、修复问题，根据社区需求新增功能<br>Long-term active maintenance. Underlying optimization, bug fixes and new features continuously added. |

## 🗺️ 算法与地形生成特点 | Algorithm & Terrain Generation
**中文**
BambooMC 搭载升级优化的程序化地形生成算法：
- 生物群系平滑混合算法，地貌过渡自然
- 实现山脉、洞穴、岩层、水系等自然地形结构
- 支持自定义噪点、地形高度、地貌风格等生成参数
- **异步区块生成**，加载地形不会阻塞服务端主线程
- 生成速度快，地形自定义程度高

**English**
BambooMC uses upgraded procedural terrain generation algorithms:
- Smooth biome blending for natural landscape transition
- Generates mountains, caves, rock layers, rivers and natural structures
- Customizable noise parameters, terrain height and landscape styles
- **Asynchronous chunk generation**, terrain loading will not block the main thread
- Fast generation speed and highly configurable terrain

## 🚀 快速使用 | Quick Start
**中文**
1. 前往 Release 下载 `BambooMC.exe`
2. 双击程序直接启动
3. 程序自动生成配置、世界目录等文件
4. 修改配置后，开放端口即可进入游戏

> 全程无需额外部署环境，极简启动流程。

**English**
1. Download `BambooMC.exe` from Releases
2. Double-click the executable to start
3. Configuration files and world directories will be generated automatically
4. Adjust settings, open the port and players can join the server

> No extra environment setup required, extremely easy to deploy.

## 🌐 更多信息 | More Information
官方网站 / Official Website：https://bamboomc.oak-ms.top
