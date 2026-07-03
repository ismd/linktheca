type Props = {
  label: string;
  value: React.ReactNode;
};

export function SettingRow({ label, value }: Props) {
  return (
    <div className="flex items-center justify-between gap-6">
      <div className="label-sc text-muted-foreground">{label}</div>
      <div className="font-body text-ink text-right">{value}</div>
    </div>
  );
}
