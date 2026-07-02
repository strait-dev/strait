import { HugeiconsIcon } from "@hugeicons/react";
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
} from "@strait/ui/components/avatar";
import { Button } from "@strait/ui/components/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useAnalytics } from "@/hooks/analytics/use-analytics";
import { authClient } from "@/lib/auth-client";
import { LogOutIcon, SettingsOutlineIcon } from "@/lib/icons";
import { captureException, clearSentryUser } from "@/lib/sentry";
import type { AuthUser } from "@/routes/__root";

type Props = {
  user: AuthUser;
};

const HeaderUserMenu = ({ user }: Props) => {
  const navigate = useNavigate();
  const [isSigningOut, setIsSigningOut] = useState(false);
  const { trackAuth, resetUser } = useAnalytics();

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
            trackAuth("LOGOUT");
            resetUser();
            clearSentryUser();
            navigate({ to: "/login" });
          },
          onError: (ctx: { error: Error & { message?: string } }) => {
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
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button size="icon" variant="ghost" />}>
        <Avatar className="size-8">
          {user.image ? (
            <AvatarImage alt="User avatar" src={user.image} />
          ) : (
            <AvatarFallback className="text-xs">{getInitials()}</AvatarFallback>
          )}
        </Avatar>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-56" sideOffset={4}>
        <DropdownMenuGroup>
          <DropdownMenuLabel className="p-0 font-normal">
            <div className="flex items-center gap-2 px-1 py-1.5 text-left text-sm">
              <Avatar className="size-8">
                {user.image ? (
                  <AvatarImage alt="User avatar" src={user.image} />
                ) : (
                  <AvatarFallback className="text-xs">
                    {getInitials()}
                  </AvatarFallback>
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
            Account settings
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem disabled={isSigningOut} onClick={handleLogout}>
          {isSigningOut ? (
            <Spinner />
          ) : (
            <HugeiconsIcon className="size-4" icon={LogOutIcon} />
          )}
          {isSigningOut ? "Signing out..." : "Sign out"}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
};

export default HeaderUserMenu;
