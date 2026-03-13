import type { AuthUser } from "@/routes/__root.tsx";

import DeleteAccount from "./delete-account.tsx";
import PersonalInfo from "./personal-info.tsx";

type Props = {
  user: AuthUser;
};

const Account = ({ user }: Props) => (
  <div className="flex flex-col gap-6">
    <PersonalInfo user={user} />
    <DeleteAccount user={user} />
  </div>
);

export default Account;
