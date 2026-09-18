import { Link } from "react-router";

type Props = {
  to: string;
  children: React.ReactNode;
};

// Sibling-screen navigation in a PageHeader. Rendered as a bordered control so
// it reads as something to press, not as caption text next to the title.
export function PageNavLink({ to, children }: Props) {
  return (
    <Link to={to} className="nav-chip">
      {children}
    </Link>
  );
}
