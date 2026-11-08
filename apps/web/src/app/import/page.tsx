import { permanentRedirect } from "next/navigation";

// Adding articles moved to /add; old links keep working.
export default function ImportPage() {
  permanentRedirect("/add#bookmarks");
}
