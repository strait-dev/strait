import { useAccounts } from "@/hooks/auth/use-account";
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
  const { data: accounts } = useAccounts();

  const hasPassword = accounts?.some((a) => a.providerId === "credential");
  const twoFactorEnabled = user.twoFactorEnabled ?? false;

  return (
    <div className="flex flex-col gap-6">
      <PersonalInfo user={user} />
      {hasPassword === true && <ChangePassword />}
      {hasPassword === false && <SetPassword email={user.email} />}
      {hasPassword === true && <TwoFactorSetup enabled={twoFactorEnabled} />}
      <LinkedAccounts />
      <PasskeyManagement />
      <SessionManagement />
      <DeleteAccount user={user} />
    </div>
  );
};

export default Account;
