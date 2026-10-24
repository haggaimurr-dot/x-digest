#!/usr/bin/env python3
"""One-shot / reusable: set showFlowchart on data/posts.json using shared heuristics.

Rules (BOTH required; unsure → false):
  (a) body names a concrete product/tool/tech
  (b) has usage steps / how-it-works / setup flow
Skip 观点/新闻/闲聊/吐槽 and similar.
"""
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
POSTS = ROOT / "data" / "posts.json"

PRODUCT_RE = re.compile(
    r"(?:"
    r"Claude(?:\s*Code|\s*Pro)?|Codex|ChatGPT|GPT-?\d|Grok\s*Bot|\bGrok\b|豆包(?:工作)?|"
    r"Open-?Science|Clipper|Typeless|叭哥说|Tripo|Blender|Unity|Remotion|ESP32|"
    r"RTX\s*Spark|Hugging\s*Face|Live\s*Link\s*Face|抖音直播伴侣|剪映|Cursor|"
    r"Antigravity|Opus(?:\s*5\.5)?|Astra|DeepSeek|Mole|Glimpse|AIHOT|Monid|Fable|"
    r"Unlimited-?OCR|Whoop|Apple\s*Health|UU\s*远程|网易\s*UU|Node\.?js|ffmpeg|"
    r"StarryBlu|DMIT|Apple\s*Pay|Auto-?Rig|Image\s*2\.5|Google\s*Flow|"
    r"Profound|REA|Next\.?js|Health\+GPT|Atypica|Sai|MCP|Skill|直播伴侣|面捕"
    r")",
    re.I,
)
ACTION_RE = re.compile(
    r"(打开|点击|选择|运行|执行|输入|登录|注册|安装|下载|配置|设置|新建|创建|"
    r"连接|授权|上传|录制|挂载|部署|接入|切换|填写|复制|启动|关闭|佩戴|获取|导入|"
    r"复现|迁移|报白|推流|"
    r"第一步|第二步|首先|然后|接着|保姆级|手把手|教程|SOP|从\s*0|按步骤)"
)
HARD_NO = re.compile(
    r"(必读合集|神帖合集|观察报告|盈利模式|到底为了什么|情绪比时长|"
    r"抱歉回复晚了|翻转法|代表作拿影响力|半年成长六点|"
    r"房车长帖|线下跑客户|"
    r"普通人收益路径|普通人 AI 赚钱老几样|别只盯硅谷模型|"
    r"放出.{0,20}Flash|月下载\s*\d|"
    r"涨粉复盘|可复制性的 X 涨粉|从小白到单篇|"
    r"冷启动贴写错了|血泪报价法|"
    r"GEO 公开课|主题式连载|"
    r"脆弱性（GEO）|论文：从公开内容)"
)
TUTORIAL_TITLE = re.compile(
    r"(教程|保姆|手把手|SOP|从\s*0|开通|安装|部署|上手|清理|开播|配置清单|迁移|实测长文|"
    r"打通|平替|开源还原|扒了|验收|指标化|万能复刻)"
)


def derive_show_flowchart(p: dict) -> bool:
    steps = p.get("steps") or []
    if len(steps) < 2:
        return False
    texts = [str(s.get("text") if isinstance(s, dict) else s or "") for s in steps]
    title = str(p.get("title") or "")
    summary = str(p.get("summary") or "")
    blob = "\n".join(
        [title, summary, str(p.get("tech") or ""), " ".join(p.get("tags") or []), *texts]
    )
    if HARD_NO.search(title) or HARD_NO.search(summary[:240]):
        return False
    if not PRODUCT_RE.search(blob):
        return False
    action_n = sum(1 for t in texts if ACTION_RE.search(t))
    tutorialish = bool(TUTORIAL_TITLE.search(title)) or action_n >= 2 or (
        action_n >= 1 and bool(PRODUCT_RE.search(title))
    )
    return bool(tutorialish)


def main() -> int:
    data = json.loads(POSTS.read_text(encoding="utf-8"))
    yes = no = 0
    for p in data.get("posts") or []:
        ok = derive_show_flowchart(p)
        p["showFlowchart"] = ok
        if p.get("isLong") and p.get("steps"):
            yes += int(ok)
            no += int(not ok)
    POSTS.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"updated {POSTS}: flowchart yes={yes} no={no} (among long posts with steps)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
