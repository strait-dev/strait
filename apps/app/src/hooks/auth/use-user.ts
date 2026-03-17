import { useMutation } from "@tanstack/react-query";
import { queryKeys } from "@/hooks/query-keys";
import { authClient } from "@/lib/auth-client";

/** Parameters for updating user information. */
type UpdateUserData = {
  name?: string;
  first_name?: string;
  last_name?: string;
  email?: string;
  phone?: string | null;
};

/** Updates the current user's information. */
export const useUpdateUser = () => {
  return useMutation({
    mutationKey: queryKeys.users.update.queryKey,
    mutationFn: async (data: UpdateUserData) => {
      const name =
        data.first_name && data.last_name
          ? `${data.first_name} ${data.last_name}`
          : data.name;

      const result = await authClient.updateUser({
        name,
        ...(data.email && { email: data.email }),
      });

      if (result.error) {
        throw new Error(result.error.message || "Failed to update user");
      }

      return result.data;
    },
  });
};
