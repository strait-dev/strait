import { useMutation } from "@tanstack/react-query";
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
    mutationKey: ["users", "update"],
    mutationFn: async (data: UpdateUserData) => {
      const name =
        data.first_name && data.last_name
          ? `${data.first_name} ${data.last_name}`
          : data.name;

      const result = await authClient.updateUser({
        name,
      });

      if (result.error) {
        throw new Error(result.error.message || "Failed to update user");
      }

      return result.data;
    },
  });
};
