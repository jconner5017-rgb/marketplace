"use client";

import { useState } from "react";

/**
 * The USPS postage bought for an envelope, at its true 7 × 3 in size. EasyPost
 * may send the image turned on its side; it's turned back upright here.
 */
export default function PostageImage({ src }: { src: string }) {
  const [sideways, setSideways] = useState(false);
  return (
    <div style={{ position: "relative", width: "7in", height: "3in", overflow: "hidden" }}>
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img src={src} alt="USPS postage" referrerPolicy="no-referrer"
        onLoad={(ev) => setSideways(ev.currentTarget.naturalHeight > ev.currentTarget.naturalWidth)}
        style={sideways
          ? { position: "absolute", width: "3in", height: "7in", left: "2in", top: "-2in", transform: "rotate(90deg)" }
          : { width: "7in", height: "3in", display: "block" }} />
    </div>
  );
}
