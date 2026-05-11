import { useState } from "react";
import { useNavigate } from "react-router";
import { RegisterForm } from "@/features/auth/components/RegisterForm";
import { RegistrationDisabled } from "@/features/auth/components/RegistrationDisabled";

export default function RegisterRoute() {
  const [disabled, setDisabled] = useState(false);
  const navigate = useNavigate();

  if (disabled) return <RegistrationDisabled />;

  return (
    <RegisterForm
      onSuccess={() => {
        navigate("/library", { replace: true });
      }}
      onRegistrationDisabled={() => setDisabled(true)}
    />
  );
}
