import { PageHeader } from "@/shared/layout/PageHeader";
import { AccountSection } from "@/features/settings/components/AccountSection";
import { AboutSection } from "@/features/settings/components/AboutSection";

export default function SettingsRoute() {
  return (
    <div>
      <PageHeader title="Settings" subtitle="This instance and your account." />
      <AccountSection />
      <AboutSection />
    </div>
  );
}
