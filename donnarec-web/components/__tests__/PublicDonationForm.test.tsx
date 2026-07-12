// @vitest-environment jsdom
//
// PublicDonationForm gating + bilingual behavior (plan 06-06, Task 2).
//
// These four behaviors are the unit-level proof for the public donation form:
//   1. Submit is DISABLED until all required fields are valid AND a slip is
//      attached AND consent is checked AND a Turnstile token is present.
//   2. With everything else complete but NO slip, the slip-required copy
//      surfaces (mandatory in Flow B, D-80) and submit stays disabled.
//   3. The locale (single source of truth, FR-06) drives BOTH the rendered
//      labels AND the donor_language value sent on submit.
//   4. A successful submit swaps in-page to the confirmation showing the
//      returned reference number (no navigation, no query string, D-84/D-86).
//
// The LocaleSwitcher itself toggles locale via a server action + cookie +
// server re-render, which cannot run in a unit test — so behavior 3 exercises
// the same single-source-of-truth contract by rendering the form under each
// locale and asserting labels + submitted donor_language track it.

import "@testing-library/jest-dom/vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NextIntlClientProvider } from "next-intl";

import thMessages from "@/messages/th.json";
import enMessages from "@/messages/en.json";
import { PublicDonationForm } from "@/components/PublicDonationForm";

// Fake Turnstile: a button that fires onSuccess with a fixed token when clicked.
vi.mock("@marsidev/react-turnstile", () => ({
  Turnstile: ({ onSuccess }: { onSuccess?: (token: string) => void }) => (
    <button
      type="button"
      data-testid="turnstile-fake"
      onClick={() => onSuccess?.("test-turnstile-token")}
    >
      verify
    </button>
  ),
}));

// The form owns an in-page state swap; it never navigates. Still, mock
// next/navigation defensively so any transitive import resolves.
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), refresh: vi.fn(), replace: vi.fn() }),
}));

const esc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

type Messages = typeof thMessages;

function renderForm(locale: "th" | "en") {
  const messages = locale === "th" ? thMessages : enMessages;
  return render(
    <NextIntlClientProvider
      locale={locale}
      messages={messages}
      timeZone="Asia/Bangkok"
    >
      <PublicDonationForm />
    </NextIntlClientProvider>
  );
}

function fillDonorFields(messages: Messages) {
  const f = messages.fields;
  fireEvent.change(screen.getByLabelText(new RegExp(esc(f.donorName))), {
    target: { value: "สมชาย ใจดี" },
  });
  fireEvent.change(screen.getByLabelText(new RegExp(esc(f.nationalId))), {
    target: { value: "1234567890123" },
  });
  fireEvent.change(screen.getByLabelText(new RegExp(esc(f.address))), {
    target: { value: "123 ถนนสุขุมวิท กรุงเทพ 10110" },
  });
  fireEvent.change(screen.getByLabelText(new RegExp(esc(f.amount))), {
    target: { value: "500" },
  });
  fireEvent.change(screen.getByLabelText(new RegExp(esc(f.donatedAt))), {
    target: { value: "2020-06-15" },
  });
}

function attachSlip(container: HTMLElement) {
  const fileInput = container.querySelector(
    'input[type="file"]'
  ) as HTMLInputElement;
  const file = new File(["fake-bytes"], "slip.png", { type: "image/png" });
  return userEvent.upload(fileInput, file);
}

function submitButton(messages: Messages) {
  return screen.getByRole("button", {
    name: new RegExp(esc(messages.publicDonation.submit)),
  });
}

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            data: { reference_number: "REF-ABCD1234", status: "pending_review" },
          }),
          { status: 201, headers: { "content-type": "application/json" } }
        )
    )
  );
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

describe("PublicDonationForm gating + bilingual behavior", () => {
  it("keeps submit disabled until fields + slip + consent + token are all satisfied", async () => {
    const user = userEvent.setup();
    const { container } = renderForm("th");

    // Nothing filled → disabled.
    expect(submitButton(thMessages)).toBeDisabled();

    // Valid fields only → still disabled (no slip/consent/token).
    fillDonorFields(thMessages);
    await waitFor(() => expect(submitButton(thMessages)).toBeDisabled());

    // + slip → still disabled (no consent/token).
    await attachSlip(container);
    await waitFor(() => expect(submitButton(thMessages)).toBeDisabled());

    // + consent → still disabled (no token).
    await user.click(screen.getByRole("checkbox"));
    await waitFor(() => expect(submitButton(thMessages)).toBeDisabled());

    // + Turnstile token → now enabled.
    await user.click(screen.getByTestId("turnstile-fake"));
    await waitFor(() => expect(submitButton(thMessages)).toBeEnabled());
  });

  it("surfaces the slip-required copy when everything else is complete but no slip", async () => {
    const user = userEvent.setup();
    renderForm("th");

    fillDonorFields(thMessages);
    await user.click(screen.getByRole("checkbox"));
    await user.click(screen.getByTestId("turnstile-fake"));

    // Slip is mandatory (D-80): copy shown, submit still blocked.
    expect(
      await screen.findByText(thMessages.publicDonation.slipRequired)
    ).toBeInTheDocument();
    expect(submitButton(thMessages)).toBeDisabled();
  });

  it("lets the locale drive both the rendered labels and the submitted donor_language", async () => {
    const user = userEvent.setup();
    const { container } = renderForm("en");

    // English UI is rendered (labels track the locale).
    expect(
      screen.getByRole("heading", {
        name: new RegExp(esc(enMessages.publicDonation.pageTitle)),
      })
    ).toBeInTheDocument();

    fillDonorFields(enMessages);
    await attachSlip(container);
    await user.click(screen.getByRole("checkbox"));
    await user.click(screen.getByTestId("turnstile-fake"));
    await waitFor(() => expect(submitButton(enMessages)).toBeEnabled());
    await user.click(submitButton(enMessages));

    const fetchMock = global.fetch as unknown as ReturnType<typeof vi.fn>;
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const body = fetchMock.mock.calls[0][1].body as FormData;
    expect(body.get("donor_language")).toBe("en");
    expect(body.get("donor_tax_id")).toBe("1234567890123");
    expect(body.get("turnstile_token")).toBe("test-turnstile-token");
    expect(body.get("slip")).toBeInstanceOf(File);
  });

  it("swaps in-page to the reference-number confirmation on a successful submit", async () => {
    const user = userEvent.setup();
    const { container } = renderForm("th");

    fillDonorFields(thMessages);
    await attachSlip(container);
    await user.click(screen.getByRole("checkbox"));
    await user.click(screen.getByTestId("turnstile-fake"));
    await waitFor(() => expect(submitButton(thMessages)).toBeEnabled());
    await user.click(submitButton(thMessages));

    // Confirmation swapped in — heading + reference number visible.
    expect(
      await screen.findByText(thMessages.publicDonation.confirmation.heading)
    ).toBeInTheDocument();
    expect(screen.getByText("REF-ABCD1234")).toBeInTheDocument();

    // The form (and its submit button) is gone — an in-page swap, not a route.
    expect(
      screen.queryByRole("button", {
        name: new RegExp(esc(thMessages.publicDonation.submit)),
      })
    ).not.toBeInTheDocument();
  });
});
