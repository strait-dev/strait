import type { AuthUser } from "@/routes/__root";

import DeleteAccount from "./delete-account";
import PersonalInfo from "./personal-info";

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
