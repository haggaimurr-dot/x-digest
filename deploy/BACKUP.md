# 备份与恢复演练

## 安装（服务器，root）

```bash
chmod 750 /opt/xdigest/deploy/xdigest-backup.sh
cp /opt/xdigest/deploy/xdigest-backup.service /opt/xdigest/deploy/xdigest-backup.timer /etc/systemd/system/
systemctl daemon-reload && systemctl enable --now xdigest-backup.timer
systemctl start xdigest-backup.service   # 立即手动跑一次
journalctl -u xdigest-backup.service -n 20
```

- 数据库：每天 04:30 `mysqldump --single-transaction` 压缩到 `/var/backups/xdigest/db/xdigest-时间戳.sql.gz`，保留 14 天。
- 附件：`rsync` 镜像到 `/var/backups/xdigest/attachments/`，被删除的文件移到 `attachments-deleted/时间戳/` 保留 14 天。
- 数据库账号密码从 `/etc/xdigest/api.env` 的 `XDIGEST_DSN` 读取，通过临时 600 权限的 defaults 文件传给 mysqldump，不出现在命令行或脚本里。

## 恢复演练（在测试库进行，不动生产库）

```bash
# 1. 建一个空的测试库
mysql -e "CREATE DATABASE xdigest_restore CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
# 2. 导入最近一份备份
zcat "$(ls -t /var/backups/xdigest/db/*.sql.gz | head -1)" | mysql xdigest_restore
# 3. 对比条数（应与生产库一致，备份之后新增的除外）
for t in posts post_steps post_tags authors replicas replica_steps attachments repos yt_videos bili_videos; do
  printf '%-16s prod=%s restore=%s\n' "$t" \
    "$(mysql -N -e "SELECT COUNT(*) FROM xdigest.$t")" \
    "$(mysql -N -e "SELECT COUNT(*) FROM xdigest_restore.$t")"
done
# 4. 附件：抽查几个文件可读
ls /var/backups/xdigest/attachments | head
# 5. 清理
mysql -e "DROP DATABASE xdigest_restore"
```

正式恢复：停 `xdigest-api` → 用第 2 步导入 `xdigest` 库 → `rsync -a /var/backups/xdigest/attachments/ /var/lib/xdigest/attachments/` → 启动服务 → 访问 `/api/v1/healthz`。
