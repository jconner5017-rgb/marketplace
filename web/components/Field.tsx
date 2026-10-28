type Props = {
  label: string;
  name: string;
  error?: string;
  hint?: string;
  children?: React.ReactNode;
} & React.InputHTMLAttributes<HTMLInputElement>;

/** Labeled input with its error message tied to it for screen readers. */
export default function Field({ label, name, error, hint, children, className, ...input }: Props) {
  const describedBy = [error ? `${name}-error` : null, hint ? `${name}-hint` : null].filter(Boolean).join(" ") || undefined;
  return (
    <div className={className}>
      <label htmlFor={name} className="block text-[14px] font-semibold">{label}</label>
      {children ?? (
        <input
          id={name}
          name={name}
          aria-invalid={error ? true : undefined}
          aria-describedby={describedBy}
          className={`mt-1 w-full rounded-md border px-3 py-2 ${error ? "border-slab" : "border-line focus:border-ink"}`}
          {...input}
        />
      )}
      {hint && !error && <p id={`${name}-hint`} className="mt-1 text-[12px] text-muted">{hint}</p>}
      {error && <p id={`${name}-error`} className="mt-1 text-[13px] text-slab">{error}</p>}
    </div>
  );
}
