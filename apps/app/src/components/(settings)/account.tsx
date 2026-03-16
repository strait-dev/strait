import { useEffect, useState } from "react";
import { authClient } from "@/lib/auth-client";
import type { AuthUser } from "@/routes/__root";
import ChangePassword from "./change-password";
import DeleteAccount from "./delete-account";
import LinkedAccounts from "./linked-accounts";
import PasskeyManagement from "./passkey-management";
import PersonalInfo from "./personal-info";
import SessionManagement from "./session-management";
import SetPassword from "./set-password";
import TwoFactorSetup from "./two-factor-setup";

type Props = {
  user: AuthUser;
};

const Account = ({ user }: Props) => {
  const [hasPassword, setHasPassword] = useState<boolean | null>(null);
  const [twoFactorEnabled, setTwoFactorEnabled] = useState(false);

  useEffect(() => {
    const checkAccounts = async () => {
      try {
        const result = await authClient.listAccounts();
        if (result.data) {
          const hasCredential = result.data.some(
            (a) => a.providerId === "credential"
          );
          setHasPassword(hasCredential);
        }
      } catch {
        setHasPassword(null);
      }
    };

    checkAccounts();
  }, []);

  useEffect(() => {
    setTwoFactorEnabled(user.twoFactorEnabled ?? false);
  }, [user]);

  const handleTwoFactorStatusChange = () => {
    setTwoFactorEnabled((prev) => !prev);
  };

  return (
    <div className="flex flex-col gap-6">
      <PersonalInfo user={user} />
      {hasPassword === true && <ChangePassword />}
      {hasPassword === false && <SetPassword email={user.email} />}
      {hasPassword === true && (
        <TwoFactorSetup
          enabled={twoFactorEnabled}
          onStatusChange={handleTwoFactorStatusChange}
        />
      )}
      <LinkedAccounts />
      <PasskeyManagement />
      <SessionManagement />
      <DeleteAccount user={user} />
    </div>
  );
};

export default Account;
