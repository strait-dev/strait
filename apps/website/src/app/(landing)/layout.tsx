import Footer from "./components/common/footer/footer.tsx";
import Header from "./components/common/header/header.tsx";

type Props = {
  children: React.ReactNode;
};

const Layout = ({ children }: Props) => (
  <>
    <Header />
    {children}
    <Footer />
  </>
);

export default Layout;
