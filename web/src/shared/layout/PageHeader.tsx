type Props = {
  title: string;
  subtitle?: string;
  actions?: React.ReactNode;
};

export function PageHeader({ title, subtitle, actions }: Props) {
  return (
    <header className="px-4 lg:px-8 pt-10 pb-6 border-b border-rule">
      <div className="flex items-end justify-between gap-6 flex-wrap">
        <div>
          <h1 className="display-tight text-5xl lg:text-6xl text-ink">{title}</h1>
          {subtitle && <p className="label-sc mt-3 text-muted-foreground">{subtitle}</p>}
        </div>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </div>
    </header>
  );
}
