// Help center articles. Each article is a list of blocks so pages stay
// consistent. To add or edit an article, change it here and push.

export type Block =
  | { p: string }
  | { h: string }
  | { list: string[] }
  | { note: string };

export type Article = {
  slug: string;
  title: string;
  section: "Buying" | "Selling" | "Support" | "Help & safety" | "About";
  summary: string;
  body: Block[];
  draft?: boolean; // shown with a "being written" note
};

export const ARTICLES: Article[] = [
  // ---------------- Buying ----------------
  {
    slug: "registration",
    title: "Registering an account",
    section: "Buying",
    summary: "One free account to buy, watch cards, message sellers and sell.",
    body: [
      { p: "You need an account to buy, add cards to your cart or Watchlist, message a seller, or sell. Browsing and searching don't need one." },
      { h: "Create your account" },
      { list: [
        "Choose Register at the top of any page.",
        "Enter your email, a username buyers and sellers will see, and a password of at least 10 characters.",
        "You're signed in right away, and we email you a link to confirm your address.",
        "Open the link to confirm. Until you do, you can browse, watch cards and fill your cart, but not buy, bid, make offers, message sellers or start selling.",
        "No email? Check your spam folder, or use Send a new link in the yellow bar at the top of the page.",
      ] },
      { h: "Forgot your password?" },
      { p: "Choose Forgot password? on the sign-in page and enter your email. The reset link works for one hour, and setting a new password signs you out on every device." },
      { h: "Email settings" },
      { p: "Under your name at the top of the page, Email settings turns announcement emails on or off. Account emails, like password resets, always come through." },
      { h: "Who can register" },
      { p: "Slabs & Singles serves buyers and sellers in the United States. Cards ship to US addresses only." },
    ],
  },
  {
    slug: "buying",
    title: "How buying works",
    section: "Buying",
    summary: "Prices include shipping, you pay each seller directly through Stripe, and your card issuer protects the purchase.",
    body: [
      { h: "The price is the price" },
      { p: "Every card ships free. The price on the listing is what you pay, before any sales tax." },
      { h: "Buying from several sellers" },
      { p: "Your cart groups cards by seller. Cards from the same seller ship together in one package, and many sellers give a multi-card discount, shown in your cart. Each seller is paid separately, so your card statement shows one charge per seller." },
      { h: "Graded and raw cards" },
      { list: [
        "Graded cards show the grading company, grade and certification number.",
        "Raw cards show a condition: Near Mint (NM), Lightly Played (LP), Moderately Played (MP), Heavily Played (HP) or Damaged (DMG).",
      ] },
      { h: "Watchlist" },
      { p: "Select Add to Watchlist on any listing to keep track of it. Cards that sell stay on your Watchlist, marked unavailable, until you remove them." },
      { note: "Checkout is being built. Until it opens, you can fill your cart and Watchlist." },
    ],
  },
  {
    slug: "auctions-offers",
    title: "Auctions and offers",
    section: "Buying",
    summary: "How bidding, Buy It Now on auctions, reserves and Best Offer work.",
    body: [
      { h: "Bidding" },
      { list: [
        "Enter the most you're willing to pay. We bid for you, one step at a time, only as much as needed to keep you ahead.",
        "If someone else's maximum is higher, the price rises to just above yours and you're told you've been outbid.",
        "Steps: 5¢ under $1, 25¢ to $5, 50¢ to $25, $1 to $100, $2.50 to $250, $5 to $500, $10 to $1,000.",
        "Bids can't be taken back, so bid only what you mean to pay.",
      ] },
      { h: "How auctions end" },
      { list: [
        "Auctions run 5, 7 or 10 days.",
        "A bid in the last 2 minutes adds 2 minutes, so last-second sniping doesn't decide the winner.",
        "If the price reaches the seller's limit, the auction ends right away and the bidder who got there first wins.",
        "Some auctions have a hidden reserve price. If bidding ends below it, the card doesn't sell.",
        "Some auctions offer Buy It Now until the first bid.",
      ] },
      { h: "If you win" },
      { p: "The card is held for you under Purchases. Pay within 4 days. Unpaid wins are cancelled and count against your account." },
      { h: "Best Offer" },
      { list: [
        "Listings that say “or Best Offer” let you offer less than the price.",
        "You can make up to 3 offers on a listing. Each lasts 48 hours.",
        "The seller can accept, decline or counter. Some sellers set rules that answer offers instantly.",
        "If your offer is accepted, the card is held for you. Pay within 48 hours.",
      ] },
    ],
  },
  {
    slug: "sales-events",
    title: "Sales and events",
    section: "Buying",
    summary: "Seasonal sales, release-day events and promotions.",
    draft: true,
    body: [{ p: "Upcoming sales and events will be posted here and in Announcements." }],
  },

  // ---------------- Selling ----------------
  {
    slug: "selling",
    title: "How to sell",
    section: "Selling",
    summary: "Connect Stripe, list your cards, and ship within three business days.",
    body: [
      { h: "1. Set up payments" },
      { p: "Go to Sell and connect a Stripe account. Stripe asks for your bank details and ID. Your sales are paid straight into your own Stripe account; Slabs & Singles never holds your money." },
      { h: "2. List your cards" },
      { list: [
        "List one card at a time with List a card.",
        "Upload many at once with a CSV file.",
        "Or connect FireCardManager to list your inventory and keep it in sync with eBay.",
      ] },
      { h: "3. Price with shipping included" },
      { p: "Every listing ships free, so build shipping into your price. The listing form shows what you'll receive after fees and an estimated label cost." },
      { h: "4. Ship on time" },
      { p: "Ship within three business days of the sale. Late shipping counts against your seller level." },
    ],
  },
  {
    slug: "fees",
    title: "Selling fees",
    section: "Selling",
    summary: "Listing is free. One fee per card sold: {fee.flat} up to {fee.threshold}, then {fee.percent} capped at {fee.cap}.",
    body: [
      { h: "Our fee" },
      { list: [
        "Listing is free.",
        "Cards that sell for {fee.threshold} or less: {fee.flat} each.",
        "Cards over {fee.threshold}: {fee.percent} of the sale price, never less than {fee.flat} and capped at {fee.cap}.",
        "The fee applies to the final auction price or the accepted offer, the same way.",
      ] },
      { h: "Stripe's fee" },
      { p: "Stripe charges its card processing fee on each sale, usually 2.9% + 30¢ per order for US cards. It comes out of your own Stripe balance, not through us." },
      { h: "Shipping" },
      { p: "Every listing ships free to the buyer, so shipping is part of your price." },
      { list: [
        "Stamped envelope: one raw card up to $20, with your own First-Class stamp. Tracked envelopes print free from the order.",
        "Labels: bought from the order at the carrier's price, charged to the card saved under Sell → Shipping, plus 2.9% + 30¢ card processing. For example, a $5.86 USPS Ground Advantage label costs $6.35.",
        "Typical USPS Ground Advantage prices: a bubble mailer about $5–7, a small box with a slab about $6–9, depending on distance and weight.",
        "You can also buy a label elsewhere and enter its tracking number on the order.",
      ] },
    ],
  },
  {
    slug: "feedback",
    title: "How feedback works",
    section: "Buying",
    summary: "Ratings both ways, hidden until both sides rate, with the last 12 months counting most.",
    body: [
      { p: "After an order, buyers rate sellers positive, neutral or negative, plus 1 to 5 stars for item accuracy, shipping and packaging, and communication. Sellers rate buyers positive or neutral. Sellers can't leave negatives, so nobody gets a low score for asking for a refund." },
      { h: "Hidden until you both rate" },
      { p: "Neither side sees the other's feedback until both have left theirs, or until 14 days after delivery. That stops anyone holding a rating hostage." },
      { h: "Scores" },
      { list: [
        "Feedback score: all positives minus all negatives.",
        "Positive percentage: positives out of positives plus negatives over the last 12 months, so an old mistake doesn't follow a seller forever.",
        "Every rating comes from a paid order, marked Verified purchase.",
      ] },
      { h: "Before leaving a negative" },
      { p: "Message the seller or report the problem first; most issues get fixed fast. A negative then waits 3 days before it locks, and you can change it to neutral or positive in that time. A brand-new seller's first negative (within their first 10 sales) is checked by our support team before it shows." },
      { h: "Rules" },
      { p: "Feedback can't be traded for money. Comments asking for a refund in exchange for a rating, or threatening a negative, are removed and the account is flagged. Sellers can post one public reply of up to 300 characters under any neutral or negative rating." },
      { h: "Feedback from other sites" },
      { p: "Sellers can show verified feedback from eBay, TCGplayer, COMC, Etsy or Discogs from Seller hub > Feedback. Our team confirms the profile belongs to them; it shows as its own badge and never changes their score here." },
    ],
  },
  {
    slug: "seller-levels",
    title: "Seller levels and price limits",
    section: "Selling",
    summary: "New sellers list cards up to $20, then $250, then $1,000.",
    body: [
      { p: "Price limits protect buyers from new accounts selling expensive cards before they've built a track record." },
      { list: [
        "New seller: cards up to $20 each.",
        "Standard: up to $250, after 50 sales to different buyers with at least 97% positive feedback and an account 30 days old.",
        "Trusted: up to $1,000, after 150 sales to different buyers with at least 98% positive feedback and an account 90 days old.",
      ] },
      { h: "What can lower your level" },
      { p: "Chargebacks, repeated reports from buyers, late shipping, cards that sold elsewhere and couldn't ship, or envelopes that never get a postal scan. If your level drops, listings above the new limit are hidden, not deleted, and come back when your level does." },
      { h: "How sales are counted" },
      { p: "We check every seller once a day, and again after new feedback or a chargeback. A sale counts once per buyer, for a card of $2 or more, once the order has shipped, and only if it wasn't refunded or lost to a chargeback. Sales to buyers who share an internet connection with you don't count. Feedback from the last 12 months sets your positive percentage." },
      { p: "If you think a change was wrong, appeal it once from Account standing." },
    ],
  },
  {
    slug: "shipping",
    title: "Shipping your cards",
    section: "Selling",
    summary: "Labels with tracking for most orders; stamped envelopes for one raw card up to $20.",
    body: [
      { h: "Purchased labels" },
      { p: "After a sale, choose Buy label on the order. Graded cards and every order of two or more cards ship with a label and tracking. Orders over $750 require a signature." },
      { h: "Stamped envelope" },
      { p: "Single-card orders of $20 or less may ship in a plain envelope with a First-Class stamp and the printed address label, which carries a tracking barcode." },
      { list: [
        "One card per envelope, always.",
        "Use a penny sleeve and a thin, flexible stiffener.",
        "No toploaders, magnetic holders or slabs. Rigid envelopes can't go through postal machines, may be returned or surcharged, and won't get tracking scans.",
        "Envelope tracking shows processing scans only. It does not confirm delivery.",
      ] },
      { p: "Step-by-step guides are in the Support section of the help center: Printing tracked envelopes, and Buying and printing shipping labels." },
    ],
  },
  {
    slug: "shipping-labels",
    title: "Buying and printing shipping labels",
    section: "Support",
    summary: "Set up your ship-from address and card once, then buy a tracked label from any order.",
    body: [
      { h: "One-time setup" },
      { list: [
        "Go to Sell → Shipping.",
        "Ship-from address: enter it and choose Save address. We check it with the carrier; it prints as the return address on labels and tracked envelopes.",
        "Credit card for labels: choose Add a card. Your card is stored by Stripe, not by us.",
      ] },
      { h: "Buy a label" },
      { list: [
        "Open the order (Sell → Orders) and choose Buy label.",
        "Pick a rate. You pay the carrier's price plus card processing; it's separate from what the buyer paid you.",
        "Choose Print label. The order is marked shipped and the buyer gets the tracking number automatically.",
        "Orders over $750 are sent with signature confirmation.",
      ] },
      { h: "Printing labels" },
      { list: [
        "Labels are 4 × 6 in. Print on a thermal label printer, or on plain paper and tape it on with clear tape.",
        "Print at 100% scale with margins set to None so the barcode scans.",
        "Don't tape over the barcode with shiny tape that wrinkles.",
      ] },
      { h: "Made a mistake?" },
      { p: "Choose Void label on the order before the carrier scans the package. The label is cancelled and the cost goes back to your card. Then buy a new one." },
    ],
  },
  {
    slug: "packing",
    title: "Packing cards: envelope, bubble mailer or box",
    section: "Support",
    summary: "Which package to choose, the sizes our labels use, and how to pack each so cards arrive safe.",
    body: [
      { p: "Each listing names a package. It sets how the order ships and the label size we buy. If an order mixes items, it goes in the biggest one: any box item means a box." },
      { h: "Stamped envelope" },
      { list: [
        "One raw card up to $20, in a #10 or #6¾ letter envelope with a First-Class stamp.",
        "Penny sleeve and a thin, flexible stiffener. No toploaders, magnetic holders or slabs: rigid envelopes can't go through postal machines.",
        "Print a tracked envelope from the order so the buyer gets USPS scans.",
      ] },
      { h: "Bubble mailer (7¼ × 5¼ × ¾ in)" },
      { list: [
        "For raw cards up to about 10 per order.",
        "Penny sleeve, then a toploader or semi-rigid holder. Tape the top so the card can't slide out.",
        "Sandwich between two pieces of cardboard a little larger than the toploader, and tape the edges.",
        "Label bought on the order, usually USPS Ground Advantage with tracking. We add 1 oz for the mailer.",
      ] },
      { h: "Small box (8 × 6 × 2 in)" },
      { list: [
        "For graded slabs, several toploaders, or more than 10 cards.",
        "Wrap each slab in bubble wrap and fill the gaps so nothing moves when you shake the box.",
        "Orders over $750 ship with signature confirmation.",
        "We add 4 oz for the box and padding.",
      ] },
      { h: "Weight" },
      { p: "Enter the weight of each card as you'll pack it: a raw card in a toploader is about 1 oz, a graded slab about 3–4 oz. We add the mailer or box. A kitchen scale is accurate enough; a label bought on too little weight can be charged extra by the carrier." },
      { note: "Your price includes shipping, since every listing ships free. The listing form shows a label estimate so you can see what you'll keep." },
    ],
  },
  {
    slug: "firecardmanager",
    title: "Connecting FireCardManager",
    section: "Support",
    summary: "List your FireCardManager inventory here, and keep eBay and Slabs & Singles in step.",
    body: [
      { h: "Connect" },
      { list: [
        "Set up payments first: Sell → Set up payments with Stripe.",
        "Go to Sell → Connect FCM, create an API key and paste it into FireCardManager under Settings → Slabs & Singles.",
        "In FireCardManager, open Slabs & Singles, select cards and choose List selected. Cards already listed here (SF or Both in the Status column) are skipped; use Update price/details to change them.",
      ] },
      { h: "While it's running" },
      { list: [
        "FireCardManager checks for sales every few minutes while it's open. A card that sells here moves to its Sold Log and its eBay listing is ended; a card that sells on eBay is taken off here.",
        "A card that sold here is never put back up by FireCardManager, even if it's selected again before the sale reaches it.",
      ] },
      { h: "When FireCardManager stops" },
      { p: "If the trial or license ends, or the computer is off, FireCardManager can't sync. Your eBay and Slabs & Singles listings stay live, so a card could sell twice." },
      { list: [
        "We pause the cards FireCardManager listed here after 3 days without a sync, and email you. They come back on its next sync.",
        "eBay listings aren't paused by us: end them on eBay if FireCardManager will be off for a while.",
        "To keep a card for sale here without FireCardManager, edit it on the website.",
      ] },
      { h: "Price limit for new sellers" },
      { p: "New sellers can list cards up to $20 each. The limit rises to $250, then $1,000, as you complete sales with good feedback. FireCardManager tells you which cards are over your limit; they stay on eBay." },
    ],
  },
  {
    slug: "tracked-envelopes",
    title: "Printing tracked envelopes",
    section: "Support",
    summary: "Print a USPS-barcoded envelope for orders of one raw card up to $20, so the buyer can follow it.",
    body: [
      { p: "Envelope orders can carry a USPS Intelligent Mail barcode. Each one is unique to the order. As the envelope passes through USPS sorting machines, each scan shows on the order for you and the buyer. You don't need a USPS account; the barcode is ours." },
      { h: "Print it" },
      { list: [
        "Open the order (Sell → Orders) and choose Print tracked envelope.",
        "Pick a size: #10 envelope, #6¾ envelope (fed sideways) or a 4 × 2 in label to stick on any envelope.",
        "Print, add a First-Class stamp, and mail it.",
        "Back on the order, choose \u201cSent it in a stamped envelope\u201d. The barcode becomes the order's tracking.",
      ] },
      { p: "Reprinting gives the same barcode, so a smudged envelope can simply be printed again. Your ship-from address (Sell → Shipping) prints as the return address." },
      { h: "Print dialog settings" },
      { p: "Browsers add their own margins, page titles and scaling, which move the address and can shrink the barcode until USPS can't read it. In Chrome's print dialog, open More settings and set these once; Chrome remembers them." },
      { list: [
        "Paper size: the envelope you're using (Envelope #10, Envelope #6¾, or a custom 3.625 × 6.5 in), or your label size.",
        "Margins: None.",
        "Scale: Default (100%), never \u201cFit to page\u201d.",
        "Headers and footers: off. Otherwise the page title and page number print on the envelope.",
      ] },
      { note: "Test on plain paper cut to the envelope's size first. If the text sits in the wrong place, the paper size in the dialog doesn't match the envelope." },
      { h: "#6¾ envelopes fed sideways" },
      { p: "The #6¾ layout prints on a page the width of the envelope's short side, with the address turned a quarter turn, for printers that take envelopes short edge first. If the address comes out upside down, choose \u201cturned left\u201d on the print page and print again." },
      { h: "Getting scans" },
      { list: [
        "Keep it flat and flexible: one card in a penny sleeve with a thin stiffener. Rigid or lumpy envelopes are hand-sorted and get no scans.",
        "Keep the barcode clear: no tape, stickers or handwriting over it, and nothing within about ¼ inch of it.",
        "Scans usually start the evening it's mailed or the next morning. The order checks for new scans every hour.",
        "Letters don't get a delivery scan. The last scan is usually \u201cSorted for delivery\u201d, when it's with the carrier's route.",
        "Some envelopes get few or no scans even when everything is right. That doesn't mean it's lost.",
      ] },
    ],
  },
  {
    slug: "bulk-upload",
    title: "Bulk upload and FireCardManager",
    section: "Selling",
    summary: "List up to 5,000 cards from a CSV file, or sync your inventory from FireCardManager.",
    body: [
      { h: "CSV upload" },
      { list: [
        "Download the template from Upload a CSV in the seller hub.",
        "Give every row your own SKU in external_id. Uploading the same file again updates those cards instead of listing them twice.",
        "Rows with problems are listed with what to fix; the rest are listed right away.",
      ] },
      { h: "FireCardManager" },
      { p: "Connect FireCardManager to list your inventory here for free and keep it in sync with eBay. When a card sells on either marketplace, it's removed from the other." },
    ],
  },

  // ---------------- Help & safety ----------------
  {
    slug: "returns",
    title: "Problems with an order",
    section: "Help & safety",
    summary: "Message the seller first. Sellers handle refunds; your card issuer handles disputes.",
    body: [
      { h: "1. Contact the seller" },
      { p: "Most problems are solved quickly. Open the order or the listing and choose Contact seller." },
      { h: "2. Refunds" },
      { p: "Sellers issue refunds from their own Stripe account. Slabs & Singles doesn't hold payments, so we can't send refunds ourselves." },
      { h: "3. Card disputes" },
      { p: "If a seller doesn't resolve a real problem, you can dispute the charge with your card issuer. The seller answers with the order record, tracking and your messages." },
      { h: "Reporting a member" },
      { p: "Report anyone who misleads buyers, ships something different from the listing, or asks to be paid outside the site. We review every report and can limit or close accounts." },
    ],
  },
  {
    slug: "safety",
    title: "Staying safe",
    section: "Help & safety",
    summary: "Always pay on Slabs & Singles. Never pay through Venmo, Zelle or PayPal Friends & Family.",
    body: [
      { p: "Paying on the site is what gives you card-issuer protection. The most common card scam is a seller asking to be paid another way to “save fees.”" },
      { list: [
        "Never pay with Venmo, Zelle, Cash App or PayPal Friends & Family for a card listed here.",
        "Messages that include emails, phone numbers or payment apps aren't sent.",
        "Check a graded card's cert number with the grading company.",
        "Report anyone who asks to move the sale off the site.",
      ] },
    ],
  },
  {
    slug: "taxes",
    title: "Taxes",
    section: "Help & safety",
    summary: "Sellers are responsible for sales tax and income reporting; Stripe sends 1099-K forms.",
    body: [
      { h: "Sales tax" },
      { p: "Sellers are responsible for collecting and paying sales tax where their state requires it. Stripe Tax can calculate it on your own Stripe account." },
      { h: "Form 1099-K" },
      { p: "Stripe files Form 1099-K for sellers who meet the IRS reporting thresholds and sends you a copy. For 2026 the federal threshold is more than $20,000 and more than 200 transactions; some states use lower thresholds. Your sales may be taxable income either way." },
      { note: "This is general information, not tax advice. Talk to a tax professional about your situation." },
    ],
  },
  {
    slug: "contact",
    title: "Contact us",
    section: "Help & safety",
    summary: "How to reach the Slabs & Singles team.",
    draft: true,
    body: [
      { p: "For a problem with an order, message the seller first from the listing or your Messages." },
      { p: "A support contact form is on the way. Announcements in your notification bell will let you know when it's ready." },
    ],
  },
  {
    slug: "community",
    title: "Community guidelines",
    section: "Help & safety",
    summary: "Be honest about condition, ship what you list, and keep it friendly.",
    body: [
      { list: [
        "Describe cards honestly, with clear photos of the front and back.",
        "Ship what you listed, on time, packed to protect it.",
        "Keep messages respectful and on the site.",
        "No counterfeits, reprints sold as originals, or altered cards sold as unaltered.",
      ] },
    ],
  },

  // ---------------- About ----------------
  {
    slug: "about",
    title: "About Slabs & Singles",
    section: "About",
    summary: "A US marketplace for sports cards and TCG singles up to $1,000.",
    body: [
      { p: "Slabs & Singles is a marketplace built only for trading cards: sports cards and trading card game singles, raw or graded, priced up to $1,000. Sellers are paid directly through their own Stripe accounts, every card ships free, and listing is free." },
    ],
  },
  {
    slug: "policies",
    title: "Policies",
    section: "About",
    summary: "Terms of use, privacy, prohibited items and messaging.",
    draft: true,
    body: [
      { h: "Messages" },
      { p: "Messages between members are stored and may be reviewed to prevent fraud and settle disputes." },
      { h: "Prohibited items" },
      { p: "Counterfeit or unlicensed reproductions, cards over $1,000, and anything that isn't a trading card." },
      { p: "Full terms of use and the privacy policy will be published here before launch." },
    ],
  },
  {
    slug: "advertise",
    title: "Advertise with us",
    section: "About",
    summary: "Reach collectors and card shops.",
    draft: true,
    body: [{ p: "Advertising options are coming. Check back soon." }],
  },
  {
    slug: "developers",
    title: "Developers",
    section: "About",
    summary: "The listings API used by FireCardManager and other inventory tools.",
    body: [
      { p: "Inventory tools can create, update and remove listings through the Slabs & Singles API, keyed by the seller's own SKU." },
      { list: [
        "PUT /v1/seller/listings/external/{sku} creates or replaces a listing.",
        "DELETE /v1/seller/listings/{id} removes one.",
        "GET /v1/catalog returns the sports, games, conditions and graders the site accepts.",
        "POST /v1/photos with a JPEG, PNG or GIF as the body (Content-Type image/jpeg, image/png or image/gif) returns a photo URL to use in photo_urls. Photos are resized to 1600 pixels and their metadata, including location, is removed.",
      ] },
      { p: "Sellers create API keys at Seller hub › Connect FCM and send them as Authorization: Bearer <key>. Contact us if you build a card inventory tool." },
    ],
  },
  {
    slug: "security",
    title: "Security center",
    section: "About",
    summary: "How we protect your account and payments.",
    body: [
      { list: [
        "Card details go straight to Stripe. We never see or store them.",
        "Passwords are stored using strong one-way hashing; we can't read them.",
        "Sign-in sessions use secure, HttpOnly cookies and end when you sign out.",
        "Repeated failed sign-ins are slowed down to stop password guessing.",
      ] },
      { p: "Think you've found a security problem? Tell us through Contact us." },
    ],
  },
];

export const SECTIONS: Article["section"][] = ["Buying", "Selling", "Support", "Help & safety", "About"];

/** One line under each section heading in the help center. */
export const SECTION_INTRO: Partial<Record<Article["section"], string>> = {
  Support: "Step-by-step guides and fixes for the tools on the site.",
};

export function article(slug: string): Article | undefined {
  return ARTICLES.find((a) => a.slug === slug);
}
