# BENZHI_README

## 项目说明
- 项目：benzhi-project-3e8d3ca5-c452-4d97-ad60-fde956777a95
- 项目用途：已完整实现面向考古工地的探方回填封护验收台，覆盖基线冻结、双人方案批准、逐层施工规则检验、缺陷整改复验、独立验收、不可变档案与证据链校验。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 项目描述
- 项目名称：探方回填封护验收台
- 项目介绍：面向考古工地保护人员的探方回填封护验收应用，将已完成记录的单个探方从回填方案冻结、分层施工记录、规则检验、缺陷整改推进到独立复核和封护档案固化；按 standard 档规划不少于 2000 行真实生产 Go 代码和 20 个生产 Go 文件。
- 项目概述：面向考古工地保护人员的探方回填封护验收应用，将已完成记录的单个探方从回填方案冻结、分层施工记录、规则检验、缺陷整改推进到独立复核和封护档案固化；按 standard 档规划不少于 2000 行真实生产 Go 代码和 20 个生产 Go 文件。
- 核心工作流：探方完成发掘记录后建立封护案件并冻结回填基线，负责人批准分层方案，现场人员依次登记各层施工与检测数据，系统对超差项阻断并要求整改复验，全部层次合格后由未参与施工的验收员独立裁定，最终生成不可变封护档案并将案件转为已封存。
- 对外接口：Go 服务直接提供原生 HTML、CSS 和 JavaScript 单页工作台及仅供同源页面调用的 JSON 端点；页面以探方剖面层次、当前状态、待办动作和证据时间线为核心，不使用 Node 构建链。服务监听支持 -addr=127.0.0.1:<port>，默认 127.0.0.1:19081，绝不默认绑定 0.0.0.0 或常见低位端口。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/siteclosure -selftest -addr=127.0.0.1:19081

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-3e8d3ca5-c452-4d97-ad60-fde956777a95-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-3e8d3ca5-c452-4d97-ad60-fde956777a95-arm64 linux/arm64

docker run -it benzhi-project-3e8d3ca5-c452-4d97-ad60-fde956777a95-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/siteclosure -selftest -addr=127.0.0.1:19081`
