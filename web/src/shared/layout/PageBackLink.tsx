import { Link } from "react-router";

type Props = {
  to: string;
  children: React.ReactNode;
};

// Return to the parent screen from a PageHeader's actions slot. Kept quieter
// than the buttons beside it — the arrow carries it, so it never competes with
// the primary action.
export function PageBackLink({ to, children }: Props) {
  return (
    <Link to={to} className="back-link whitespace-nowrap mr-2">
      ← {children}
    </Link>
  );
}
