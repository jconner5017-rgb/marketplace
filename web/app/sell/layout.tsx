import { getSeller } from "@/lib/api";
import HubTabs from "@/components/sell/HubTabs";

// Every seller hub page gets the tab bar once the seller can take payments.
export default async function SellLayout({ children }: { children: React.ReactNode }) {
  const { seller } = await getSeller().catch(() => ({ seller: null }));
  return (
    <>
      {seller?.charges_enabled && <HubTabs />}
      {children}
    </>
  );
}
