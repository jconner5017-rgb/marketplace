import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import PostageTest, { type Addr } from "@/components/admin/PostageTest";

export const dynamic = "force-dynamic";
export const metadata = { title: "Test envelope postage" };

export default async function PostageTestPage() {
  await requireStaff("/admin/postage-test", "staff.manage");
  const r = await apiGet<{ configured: boolean; test_mode?: boolean; from?: Addr }>("/v1/admin/postage-test");
  const d = r.data;
  return (
    <div className="max-w-[760px]">
      <h1 className="text-[26px] font-bold">Test envelope postage</h1>
      <p className="mt-1 text-muted">
        Asks EasyPost for USPS First-Class <b>letter</b> postage on an envelope, and with the Test key buys one so you can see the label.
        Nothing here touches orders, and a Test key never charges anything.
      </p>
      {!d?.configured ? (
        <p className="mt-4 rounded-lg border border-line p-4">Add your EasyPost Test key on Render: marketplace-api &gt; Environment &gt; <code>EASYPOST_API_KEY</code>.</p>
      ) : (
        <>
          <p className={`mt-3 text-[14px] font-semibold ${d.test_mode ? "text-green-700" : "text-red-700"}`}>
            {d.test_mode ? "Using the EasyPost Test key: free test labels." : "This is a LIVE EasyPost key: rates only, buying is turned off on this page."}
          </p>
          <PostageTest from={d.from} canBuy={!!d.test_mode} />
        </>
      )}
    </div>
  );
}
