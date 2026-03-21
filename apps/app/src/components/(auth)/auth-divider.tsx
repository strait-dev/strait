type AuthDividerProps = {
  label?: string;
};

const AuthDivider = ({ label = "or" }: AuthDividerProps) => {
  return (
    <div className="relative flex items-center justify-center">
      <div className="absolute inset-0 flex items-center">
        <div className="w-full border-border border-t" />
      </div>
      <span className="relative bg-background px-2 text-muted-foreground text-xs">
        {label}
      </span>
    </div>
  );
};

export default AuthDivider;
