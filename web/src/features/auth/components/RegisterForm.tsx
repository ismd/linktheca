import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Link } from "react-router";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { Label } from "@/shared/ui/label";
import { register as registerApi } from "@/features/auth/api";
import { ApiError } from "@/shared/api/errors";

const schema = z.object({
  email: z.string().email("Please enter a valid email address"),
  displayName: z.string().min(1, "Display name is required"),
  password: z.string().min(10, "Password must be at least 10 characters"),
});

type FormValues = z.infer<typeof schema>;

type Props = {
  onSuccess: () => void;
  onRegistrationDisabled: () => void;
};

export function RegisterForm({ onSuccess, onRegistrationDisabled }: Props) {
  const [topError, setTopError] = useState<string | null>(null);
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { email: "", displayName: "", password: "" },
  });

  const onSubmit = handleSubmit(async (values) => {
    setTopError(null);
    try {
      await registerApi(values);
      onSuccess();
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 403 && err.code === "registration_disabled") {
          onRegistrationDisabled();
          return;
        }
        if (err.status === 409) {
          setTopError("An account with this email already exists.");
          return;
        }
        if (err.status === 400 && err.code === "weak_password") {
          setError("password", { message: "Password is too weak" });
          return;
        }
        if (err.status >= 500) {
          setTopError("Service unavailable. Please try again.");
          return;
        }
        setTopError(err.message || "Something went wrong");
        return;
      }
      setTopError("Something went wrong");
    }
  });

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
      {topError && (
        <div
          role="alert"
          className="border border-vermillion bg-paper-2 px-4 py-3 text-sm text-vermillion-dark"
        >
          {topError}
        </div>
      )}

      <div className="flex flex-col gap-2">
        <Label htmlFor="email" className="label-sc text-ink-3">
          Email
        </Label>
        <Input
          id="email"
          type="email"
          autoComplete="email"
          aria-invalid={errors.email ? "true" : "false"}
          aria-describedby={errors.email ? "email-error" : undefined}
          {...register("email")}
        />
        {errors.email && (
          <p id="email-error" className="text-sm text-vermillion-dark">
            {errors.email.message}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="displayName" className="label-sc text-ink-3">
          Display name
        </Label>
        <Input
          id="displayName"
          autoComplete="nickname"
          aria-invalid={errors.displayName ? "true" : "false"}
          aria-describedby={errors.displayName ? "displayName-error" : undefined}
          {...register("displayName")}
        />
        {errors.displayName && (
          <p id="displayName-error" className="text-sm text-vermillion-dark">
            {errors.displayName.message}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="password" className="label-sc text-ink-3">
          Password
        </Label>
        <Input
          id="password"
          type="password"
          autoComplete="new-password"
          aria-invalid={errors.password ? "true" : "false"}
          aria-describedby="password-help"
          {...register("password")}
        />
        <p id="password-help" className="text-sm text-muted">
          {errors.password ? (
            <span className="text-vermillion-dark">{errors.password.message}</span>
          ) : (
            "At least 10 characters."
          )}
        </p>
      </div>

      <Button type="submit" disabled={isSubmitting}>
        {isSubmitting ? "Creating account…" : "Create account"}
      </Button>

      <p className="label-sc text-center text-muted">
        <Link to="/login" className="hover:text-ink-3">
          ← Back to sign in
        </Link>
      </p>
    </form>
  );
}
