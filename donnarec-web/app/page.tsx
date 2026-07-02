import { redirect } from "next/navigation";

/**
 * Root `/` — not a page of its own; the back-office landing page is the
 * donation list. Middleware already forces authentication on `/`, so only
 * authenticated users ever reach this redirect.
 */
export default function Home() {
  redirect("/donations");
}
