import type { Metadata } from "next";
import { OnboardingUI } from "../../components/onboarding-ui";

export const metadata: Metadata = { title: "Connect to WizPay MCP", robots: { index: false, follow: false } };
export default function OnboardingPage() { return <OnboardingUI />; }
