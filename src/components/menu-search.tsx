"use client";

import * as React from "react";
import { Search } from "lucide-react";
import { useRouter } from "next/navigation";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Kbd } from "@/components/ui/kbd";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";

export interface MenuItem {
  t_key: string;
  url: string;
}

/**
 * Searchable command palette for sidebar menu items (shadcn Command / cmdk).
 * - Trigger button shown in the sidebar header
 * - Global shortcut: Ctrl/Cmd + K
 */
export function MenuSearch({ items }: { items: MenuItem[] }) {
  const { t } = useTranslation("sidebar");
  const router = useRouter();
  const [open, setOpen] = React.useState(false);

  React.useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setOpen((o) => !o);
      }
    };
    document.addEventListener("keydown", down);
    return () => document.removeEventListener("keydown", down);
  }, []);

  const label = (key: string) => t(`items.${key}`);

  const runItem = (item: MenuItem) => {
    setOpen(false);
    router.push(item.url);
  };

  return (
    <>
      <Button
        type="button"
        variant="outline"
        className="h-8 w-full justify-start gap-2 text-xs text-muted-foreground"
        onClick={() => setOpen(true)}
      >
        <Search className="size-3.5" />
        <span className="flex-1 truncate text-left">{t("search_menu")}</span>
        <Kbd>⌘K</Kbd>
      </Button>

      <CommandDialog
        open={open}
        onOpenChange={setOpen}
        title={t("search_menu")}
        description={t("search_menu_hint")}
      >
        <CommandInput placeholder={t("search_menu_placeholder")} />
        <CommandList>
          <CommandEmpty>{t("search_menu_empty")}</CommandEmpty>
          <CommandGroup heading={t("search_menu_group")}>
            {items.map((item) => (
              <CommandItem
                key={item.url}
                value={`${label(item.t_key)} ${item.url}`}
                onSelect={() => runItem(item)}
                keywords={[item.url]}
              >
                <Search className="mr-2 size-3.5" />
                <span>{label(item.t_key)}</span>
              </CommandItem>
            ))}
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    </>
  );
}