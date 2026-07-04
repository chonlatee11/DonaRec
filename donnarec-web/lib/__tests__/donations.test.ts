import { describe, it, expect, vi, beforeEach } from "vitest";
import type { SearchFilter } from "../donations";

/**
 * fetchDonations shape-hardening regression tests (Phase 3 UAT).
 *
 * Bug found in the human walkthrough: /donations threw
 *   "Cannot read properties of undefined (reading 'length')" at DonationTable
 * because a stale API served the legacy bare-array list contract `{data:[...]}`,
 * fetchDonations returned an array (no `.items`), and DonationTable crashed on
 * `items.length`. These tests lock in the defensive normalization so the list
 * never hands a non-array `items` to the table again.
 *
 * Hermetic: lib/donations imports @/lib/api which loads next-auth/next + @/lib/auth
 * at module init — mock them so this runs in the vitest `node` env with no network.
 */
vi.mock("next-auth/next", () => ({ getServerSession: vi.fn() }));
vi.mock("@/lib/auth", () => ({ authOptions: {} }));

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

// Import AFTER the mocks so the module picks them up.
const { fetchDonations } = await import("../donations");

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const filter: SearchFilter = {
  name: "",
  status: "",
  from: "",
  to: "",
  receipt_no: "",
  page: 1,
};

describe("fetchDonations shape hardening (Phase 3 UAT regression)", () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });

  it("returns items from the D-R2 paginated envelope {data:{items,total,page,per_page}}", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({ data: { items: [{ id: "1" }], total: 1, page: 1, per_page: 20 } })
    );
    const res = await fetchDonations(filter);
    expect(Array.isArray(res.items)).toBe(true);
    expect(res.items).toHaveLength(1);
    expect(res.total).toBe(1);
  });

  it("coerces the legacy bare-array contract {data:[...]} into a paginated shape (never crashes DonationTable)", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ data: [{ id: "1" }, { id: "2" }] }));
    const res = await fetchDonations(filter);
    expect(Array.isArray(res.items)).toBe(true);
    expect(res.items).toHaveLength(2);
    expect(res.total).toBe(2);
  });

  it("throws on an unexpected shape (no items array) so the UI shows an error, not a crash", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ data: { total: 0 } }));
    await expect(fetchDonations(filter)).rejects.toThrow();
  });

  it("throws on a non-ok BFF response (e.g. 401 after login) so isError renders the alert", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({ error: "unauthenticated", message: "no session" }, 401)
    );
    await expect(fetchDonations(filter)).rejects.toThrow();
  });
});
