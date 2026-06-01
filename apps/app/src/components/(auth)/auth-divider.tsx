import { FieldSeparator } from "@strait/ui/components/field";

type AuthDividerProps = {
  label?: string;
};

const AuthDivider = ({ label = "or" }: AuthDividerProps) => (
  <FieldSeparator>{label}</FieldSeparator>
);

export default AuthDivider;
