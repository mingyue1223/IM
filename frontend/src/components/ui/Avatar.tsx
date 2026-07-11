import { cn } from "../../lib/cn";

type AvatarSize = "sm" | "md" | "lg" | "xl";

export interface AvatarProps {
  src?: string;
  name: string;
  size?: AvatarSize;
  online?: boolean;
  className?: string;
}

function initials(name: string) {
  return name.trim().slice(0, 2).toUpperCase() || "?";
}

export function Avatar({ src, name, size = "md", online, className }: AvatarProps) {
  return (
    <span className={cn("ui-avatar", `ui-avatar--${size}`, className)} aria-label={name} role="img">
      {src ? <img src={src} alt="" /> : <span>{initials(name)}</span>}
      {online !== undefined && <i className={cn("ui-avatar__status", online && "is-online")} aria-hidden="true" />}
    </span>
  );
}
