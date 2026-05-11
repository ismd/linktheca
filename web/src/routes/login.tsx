import { useLocation, useNavigate } from "react-router";
import { LoginForm } from "@/features/auth/components/LoginForm";

type LocationState = { from?: { pathname?: string } } | null;

export default function LoginRoute() {
  const navigate = useNavigate();
  const location = useLocation();
  const state = location.state as LocationState;
  const from = state?.from?.pathname ?? "/library";

  return (
    <LoginForm
      onSuccess={() => {
        navigate(from, { replace: true });
      }}
    />
  );
}
