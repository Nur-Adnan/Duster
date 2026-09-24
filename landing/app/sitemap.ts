import type { MetadataRoute } from "next";

import { siteUrl } from "@/lib/content";

export default function sitemap(): MetadataRoute.Sitemap {
  // Single-page site: only the canonical home URL belongs here.
  return [
    {
      url: siteUrl.href,
      lastModified: new Date(),
    },
  ];
}
