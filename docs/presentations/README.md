# ZettaBridge × Zerodha Demo Deck

**Final presentation for Zerodha review** — July 2026  
**Goal:** Demonstrate product capability, safety, security, and transparency to support Kite Connect / Personal API approval.

---

## Files in this folder

| File | Purpose |
|------|---------|
| `ZettaBridge_Zerodha_Demo_Final.marp.md` | **Primary deck** — open in VS Code with [Marp](https://marketplace.visualstudio.com/items?itemName=marp-team.marp-vscode), export to PPTX/PDF |
| `ZettaBridge_Zerodha_Demo_Final.pptx` | **Built deck** — 22 slides with flow diagrams (python-pptx) |
| `preliminary_deck_notes.txt` | Text extracted from your two preliminary decks |
| `build_zerodha_demo_ppt.sh` | Rebuild script |
| `../scripts/build_zerodha_demo_ppt.py` | Python builder with diagrams as shapes |

---

## Quick export to PowerPoint (recommended)

### Option A — Marp (best visuals)

1. Install VS Code extension: **Marp for VS Code**
2. Open `ZettaBridge_Zerodha_Demo_Final.marp.md`
3. Click **Export Slide Deck** → choose **PowerPoint (.pptx)**

### Option B — Python script (programmatic shapes)

```bash
cd zettabridge-public
bash docs/presentations/build_zerodha_demo_ppt.sh
```

Output:
- `docs/presentations/ZettaBridge_Zerodha_Demo_Final.pptx`
- Copy to `C:\Users\Surya\Downloads\ZettaBridge_Zerodha_Demo_Final.pptx`

---

## Slide count

~22 slides covering: purpose, product overview, architecture, ingest flow, paper vs Publisher, security, data boundary, Publisher limitations, Kite Connect comparison, operational controls, and ask.

---

## Preliminary decks ingested

Content synthesized from:
- `ZettaBridge_Zerodha_Demo_Preparation.pptx`
- `ZettaBridge_Zerodha_Broker_Demo.pptx`

Plus product docs: `docs/diagrams/`, `docs/architecture-diagrams.md`, `docs/04-integrations/zerodha-publisher.md`.
