# x-digest 本地语音识别 (STT)

## 架构
- `xdigest-stt.service` (127.0.0.1:18101): Python + faster-whisper base 模型，CPU 推理
- Go 后端 `/api/v1/stt`: 接收音频 multipart，转发给本地 STT 服务
- 前端 NoteEditor: 麦克风按钮，MediaRecorder 录音 → 转录 → 插入文本（可编辑）

## 部署位置
- 服务: /opt/xdigest-stt/ (venv, stt_server.py, model/)
- 模型: /opt/xdigest-stt/model/ (faster-whisper-base, ~148MB)
- systemd: /etc/systemd/system/xdigest-stt.service

## 已知问题
- faster-whisper 1.2.1 与 PyAV 19.0.2 不兼容：`audio.py` 用了 `av.open(..., metadata_errors="ignore")`
  但 PyAV 19 不支持该参数。已手动 patch：
  `sed -i 's/av.open(input_file, mode="r", metadata_errors="ignore")/av.open(input_file, mode="r")/'`
  `/opt/xdigest-stt/venv/lib/python3.12/site-packages/faster_whisper/audio.py`
  **重建 venv 后需重新打这个 patch。**

## 性能
- base 模型：2 秒音频约 8 秒转录（4 vCPU）
- 录音最长 2 分钟（前端限制），后端限制 10MB
