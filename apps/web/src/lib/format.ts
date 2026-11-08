import { brand } from "@/config/brand";

const dateFormat = new Intl.DateTimeFormat(brand.dateLocale, { dateStyle: "medium", timeZone: "UTC" });

export function formatDate(iso: string): string {
  return dateFormat.format(new Date(iso));
}
