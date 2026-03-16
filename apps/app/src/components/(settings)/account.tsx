import { useEffect, useState } from "react";
import { authClient } from "@/lib/auth-client";
import type { AuthUser } from "@/routes/__root";
import ChangePassword from "./change-password";
import DeleteAccount from "./delete-account";
import LinkedAccounts from "./linked-accounts";
import PersonalInfo from "./personal-info";
import SetPassword from "./set-password";

type Props = {
  user: AuthUser;
};

const Account = ({ user }: Props) => {
  const [hasPassword, setHasPassword] = useState<boolean | null>(null);

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

  return (
    <div className="flex flex-col gap-6">
      <PersonalInfo user={user} />
      {hasPassword === true && <ChangePassword />}
      {hasPassword === false && <SetPassword email={user.email} />}
      <LinkedAccounts />
      <DeleteAccount user={user} />
    </div>
  );
};

export default Account;
