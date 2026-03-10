import Footer from "./components/common/footer/footer";
import Header from "./components/common/header/header";

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
