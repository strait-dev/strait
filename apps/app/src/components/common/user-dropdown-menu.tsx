import { HugeiconsIcon } from "@hugeicons/react";
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
} from "@strait/ui/components/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@strait/ui/components/sidebar";
import { toast } from "@strait/ui/components/toast/index";
import { Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useAnalytics } from "@/hooks/analytics/use-analytics";
import { authClient } from "@/lib/auth-client";
import {
  ArrowRightIcon,
  LoadingIcon,
  LogOutIcon,
  SettingsOutlineIcon,
} from "@/lib/icons";
import { captureException, clearSentryUser } from "@/lib/sentry";
import type { AuthUser } from "@/routes/__root";

type Props = {
  user: AuthUser;
};

const UserDropdownMenu = ({ user }: Props) => {
  const navigate = useNavigate();
  const [isSigningOut, setIsSigningOut] = useState(false);
  const { trackAuth, resetUser } = useAnalytics();

  // Use name field with email fallback
  const displayName = user.name || user.email.split("@")[0];

  const getInitials = () => {
    if (user.name) {
      const nameParts = user.name.split(" ");
      if (nameParts.length >= 2) {
        return `${nameParts[0].charAt(0)}${nameParts.at(-1)?.charAt(0)}`;
      }
      return user.name.charAt(0).toUpperCase();
    }
    return user.email.charAt(0).toUpperCase();
  };

  const handleLogout = async () => {
    setIsSigningOut(true);
    try {
      await authClient.signOut({
        fetchOptions: {
          onSuccess: () => {
            // Track logout event and reset PostHog user
            trackAuth("LOGOUT");
            resetUser();
            clearSentryUser();
            navigate({ to: "/login" });
          },
          onError: (ctx) => {
            captureException(ctx.error);
            toast.error("Error signing out. Please try again.");
          },
        },
      });
    } catch (error) {
      captureException(error);
      toast.error("Error signing out. Please try again.");
    } finally {
      setIsSigningOut(false);
    }
  };

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <SidebarMenuButton
                className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
                size="lg"
              />
            }
          >
            <Avatar className="size-10">
              {user.image ? (
                <AvatarImage alt="User Avatar" src={user.image} />
              ) : (
                <AvatarFallback>{getInitials()}</AvatarFallback>
              )}
            </Avatar>
            <div className="grid flex-1 text-left text-sm leading-tight">
              <span className="truncate font-medium text-sm">{user.name} </span>
              <span className="truncate text-xs">{user.email}</span>
            </div>
            <HugeiconsIcon className="ml-auto size-4" icon={ArrowRightIcon} />
          </DropdownMenuTrigger>

          <DropdownMenuContent
            align="end"
            className="w-[--radix-dropdown-menu-trigger-width] min-w-56 rounded-custom"
            side="bottom"
            sideOffset={4}
          >
            <DropdownMenuGroup>
              <DropdownMenuLabel className="p-0 font-normal">
                <div className="flex items-center gap-2 px-1 py-1.5 text-left text-sm">
                  <Avatar className="size-10">
                    {user.image ? (
                      <AvatarImage alt="User Avatar" src={user.image} />
                    ) : (
                      <AvatarFallback>{getInitials()}</AvatarFallback>
                    )}
                  </Avatar>
                  <div className="grid flex-1 text-left text-sm leading-tight">
                    <span className="truncate font-medium text-sm">
                      {displayName}
                    </span>
                    <span className="truncate text-xs">{user.email}</span>
                  </div>
                </div>
              </DropdownMenuLabel>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem
                render={<Link preload="intent" to="/app/settings" />}
              >
                <HugeiconsIcon className="size-4" icon={SettingsOutlineIcon} />
                Settings
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuItem disabled={isSigningOut} onClick={handleLogout}>
              {isSigningOut ? (
                <HugeiconsIcon
                  className="size-4 animate-spin"
                  icon={LoadingIcon}
                />
              ) : (
                <HugeiconsIcon className="size-4" icon={LogOutIcon} />
              )}
              {isSigningOut ? "Signing out..." : "Sign out"}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
};

export default UserDropdownMenu;
