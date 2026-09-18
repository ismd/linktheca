import { Link } from "react-router";
import { Button } from "@/shared/ui/button";

type Props = {
  to: string;
  children: React.ReactNode;
};

// Sibling-screen navigation in a PageHeader. Rendered as a real control so it
// reads as something to press, not as caption text next to the title.
export function PageNavLink({ to, children }: Props) {
  return (
    <Button variant="outline" asChild>
      <Link to={to}>{children}</Link>
    </Button>
  );
}
