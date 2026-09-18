import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { Slot } from "radix-ui";

import { cn } from "@/shared/lib/cn";

// The look lives in styles/globals.css under .btn — see the note there. This
// only maps variant/size names onto it, so call sites stay untouched.
const buttonVariants = cva("btn", {
  variants: {
    variant: {
      default: "btn-primary",
      destructive: "btn-destructive",
      outline: "btn-outline",
      secondary: "btn-secondary",
      ghost: "btn-ghost",
      link: "btn-link",
    },
    size: {
      default: "btn-md",
      xs: "btn-sm",
      sm: "btn-sm",
      lg: "btn-lg",
      icon: "btn-icon-md",
      "icon-xs": "btn-icon-xs",
      "icon-sm": "btn-icon-sm",
      "icon-lg": "btn-icon-lg",
    },
  },
  defaultVariants: {
    variant: "default",
    size: "default",
  },
});

function Button({
  className,
  variant = "default",
  size = "default",
  asChild = false,
  ...props
}: React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean;
  }) {
  const Comp = asChild ? Slot.Root : "button";

  return (
    <Comp
      data-slot="button"
      data-variant={variant}
      data-size={size}
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  );
}

export { Button, buttonVariants };
