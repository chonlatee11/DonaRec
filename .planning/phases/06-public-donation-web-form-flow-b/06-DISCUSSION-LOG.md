# Phase 6: Public Donation Web Form (Flow B) - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-11
**Phase:** 6-public-donation-web-form-flow-b
**Areas discussed:** Public submission seam, Donor fields + slip บน public, กันบอท/สแปม, หลังกดส่ง + ack email

---

## Public submission seam

### created_by attribution
| Option | Description | Selected |
|--------|-------------|----------|
| System user เฉพาะ | seed 1 แถว users 'public-web' → created_by = id นั้น; คง NOT NULL FK | ✓ |
| created_by nullable + source | แก้ schema ให้ null ได้; migration ใหญ่กว่า + อ่อน FK invariant Phase 3 | |

### แยกคิว Flow B (FR-08)
| Option | Description | Selected |
|--------|-------------|----------|
| เพิ่ม source column | คอลัมน์ source ('flow_a'/'flow_b'); filter/badge explicit | ✓ |
| เดาจาก created_by | ดูว่า created_by = public-web; implicit/เปราะ | |

### Public API surface
| Option | Description | Selected |
|--------|-------------|----------|
| Public route group ใน Go API เดิม | /api/public/donations ไม่ผ่าน RequireAuth (มี CAPTCHA+rate-limit); reuse service | ✓ |
| แยก service ต่างหาก | microservice / Next.js server action ตรง DB; bypass encryption+validation, overkill | |

**User's choice:** System user + source column + public route group ใน Go API เดิม (D-76/D-77/D-78)
**Notes:** เป็น public/unauthenticated seam ครั้งแรกของระบบ — ทุก /api group เดิมอยู่ใต้ RequireAuth

---

## Donor fields + slip บน public

### เลขภาษี/ปชช. (D-44 NOT NULL)
| Option | Description | Selected |
|--------|-------------|----------|
| บังคับกรอกบน public | donor กรอก 13 หลักเอง; คง schema NOT NULL, pipeline เหมือน Flow A | ✓ |
| Optional → เจ้าหน้าที่เติม | ลด friction/PII แต่ต้องแก้ schema nullable + block approve | |

### สลิป Flow B
| Option | Description | Selected |
|--------|-------------|----------|
| บังคับแนบสลิป | Flow B ไม่มีเจ้าหน้าที่เห็นเงินเข้า; reuse magic-byte+size seam | ✓ |
| Optional เหมือน Flow A | ไม่บังคับ; แต่ยืนยันเงินยากตอน review | |

### consent
| Option | Description | Selected |
|--------|-------------|----------|
| reuse D-49 + version ของ public | pattern เดิม + consent_text_version ชุด public | ✓ |
| ใช้ version เดียวกับ Flow A | ข้อความเดียวทั้ง 2 flow | |

**User's choice:** บังคับ tax ID + บังคับ slip + reuse consent snapshot กับ version public (D-79/D-80/D-81)
**Notes:** คง schema invariant เดิมให้มากสุด (NOT NULL ทั้งคู่)

---

## กันบอท/สแปม (FR-04)

### CAPTCHA provider
| Option | Description | Selected |
|--------|-------------|----------|
| Cloudflare Turnstile | privacy-first ไม่ track แบบ Google; ต้อง egress ออกเน็ต | ✓ |
| reCAPTCHA v3 | แพร่หลาย แต่ส่ง traffic ให้ Google (ขัด PDPA-first) | |
| ทำเป็น config สลับได้ | abstract verifier interface, default Turnstile | (fold เข้า D-82) |

### rate limiting
| Option | Description | Selected |
|--------|-------------|----------|
| Per-IP + CAPTCHA (defense-in-depth) | Go middleware จำกัดต่อ IP + CAPTCHA คู่กัน | ✓ |
| CAPTCHA อย่างเดียว | พึ่ง CAPTCHA ล้วน; ไม่กัน automated flood | |

**User's choice:** Turnstile (หลัง config-swappable interface) + per-IP rate limit (D-82/D-83)
**Notes:** Turnstile egress ขึ้นกับ hosting = stakeholder gate เดิม → abstract interface กันเปลี่ยน provider

---

## หลังกดส่ง + ack email (FR-05)

### post-submit UX
| Option | Description | Selected |
|--------|-------------|----------|
| ยืนยันบนจอ + reference no. | confirmation + เลขอ้างอิง (ไม่ใช่เลขใบเสร็จ) | ✓ |
| ข้อความ 'ได้รับแล้ว' เฉยๆ | ไม่มีเลขอ้างอิง | |

### ack email
| Option | Description | Selected |
|--------|-------------|----------|
| Outbox job ใหม่ ('ack_email') | ผ่าน worker/outbox เดิม; decouple, retry, ไม่ block | ✓ |
| ส่ง inline ตอน submit | fail อาจกระทบ submit / ช้า response | |

### donor status tracking
| Option | Description | Selected |
|--------|-------------|----------|
| ไม่มี (out of scope) | ack email + on-screen พอ; portal = future | ✓ |
| มีหน้าเช็คสถานะ | link ดูสถานะ; scope creep MVP | |

**User's choice:** on-screen + reference no. + ack_email outbox job + ไม่มี status tracking (D-84/D-85/D-86)
**Notes:** reference no. ≠ เลขใบเสร็จ (เกิดตอน approve เท่านั้น)

---

## Claude's Discretion

- schema รายละเอียด: source enum vs text+CHECK, วิธี seed public-web user, reference-no format, migration number (000015+)
- โครงสร้าง package Go: public handler ใน internal/donation vs subpackage ใหม่; ตำแหน่ง captcha verifier + rate-limit middleware
- rate-limit ตัวเลข default + counter storage (in-memory/DB — ยังไม่มี Redis ใน stack)
- outbox ack_email payload/dispatch
- Next.js public form route/URL + Turnstile widget + language default
- responsive audit scope (breakpoint/หน้าจอ NFR-06)

## Deferred Ideas

- Donor status tracking / portal / login (D-86)
- Donor master + dedup + auto-fill + blind index (D-43)
- สลับ/self-host CAPTCHA provider (D-82 interface พร้อม)
- e-Donation API ตรง / รายงานเชิงลึก / PKI signature (v2)
