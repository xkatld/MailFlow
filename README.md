# MailFlow - 商业化 SMTP 邮件 API 服务

> 支持 ZJMF 财务系统集成的 SMTP 负载均衡邮件服务平台，提供多服务器负载均衡、配额管理和实时监控功能。

**作者：** [xkatld](https://github.com/xkatld) | **项目地址：** [MailFlow](https://github.com/xkatld/MailFlow)

详细的**安装和使用**文档，请参考 项目 [Wiki](https://github.com/xkatld/MailFlow/wiki)。

---

## 核心特性

| 负载均衡 | API 管理 | 异步队列 | 实时监控 | ZJMF 集成 |
|:---:|:---:|:---:|:---:|:---:|
| 多 SMTP 服务器<br/>优先级+轮询 | API Key 认证<br/>套餐+自定义配额 | Redis 队列<br/>高性能发送 | 配额统计<br/>发送日志 | 销售+邮件<br/>双插件 |

**技术栈：** Go · PostgreSQL · Redis · Gin · GORM

---

## 快速开始

```bash
# 1. 一键部署（自动安装 PostgreSQL + Redis）
sudo ./deploy.sh

# 2. 访问管理后台
http://服务器IP:8080/admin
# 账号密码在 /opt/mailflow/config.yaml

# 3. 发送邮件测试
curl -X POST http://localhost:8080/api/v1/send \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"to":["test@example.com"],"subject":"测试","html":"<h1>Hello</h1>"}'
```

## 常用命令

| 命令 | 说明 | 命令 | 说明 |
|------|------|------|------|
| `systemctl start mailflow` | 启动服务 | `systemctl status mailflow` | 查看状态 |
| `systemctl stop mailflow` | 停止服务 | `journalctl -u mailflow -f` | 查看日志 |
| `systemctl restart mailflow` | 重启服务 | `sudo ./upgrade.sh` | 一键升级 |

## 系统要求

- **操作系统：** Linux（amd64/arm64）
- **数据库：** PostgreSQL 12+
- **缓存：** Redis 6+
- **编译：** Go 1.23+
