import { ApiError } from "@/shared/api/errors";
import { useRadarStatusQuery } from "@/features/radar/use-radar";
import { fmtSweep } from "@/features/radar/time";
import { APP_VERSION } from "@/shared/version";
import { SettingRow } from "./SettingRow";

function radarStatusLabel(status: ReturnType<typeof useRadarStatusQuery>): string {
  if (status.isLoading) return "Checking…";
  if (status.isSuccess) return fmtSweep(status.data.lastSweepAt);
  if (status.error instanceof ApiError && status.error.code === "radar_disabled") {
    return "Disabled";
  }
  return "Unavailable";
}

export function AboutSection() {
  const status = useRadarStatusQuery();

  return (
    <section className="px-4 lg:px-8 py-8">
      <div className="mb-6">
        <h2 className="display-tight text-2xl text-ink mb-1">About</h2>
        <p className="font-body italic text-sm text-muted-foreground">
          This instance.
        </p>
      </div>
      <div className="bg-paper-2 border border-rule p-6 md:p-8 flex flex-col gap-4">
        <SettingRow label="Version" value={`v${APP_VERSION}`} />
        <SettingRow label="Mode" value="self-hosted" />
        <SettingRow label="Radar" value={radarStatusLabel(status)} />
      </div>
    </section>
  );
}
