import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "WizPay approvals",
  description: "Human approval boundary for WizPay intents.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
