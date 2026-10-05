/** Joins truthy class-name fragments without resolving conflicting Tailwind classes. */

export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(' ');
}
