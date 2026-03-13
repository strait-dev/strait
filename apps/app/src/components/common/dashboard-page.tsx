import {
  Add01Icon,
  Analytics01Icon,
  CheckmarkCircle01Icon,
  CreditCardIcon,
  Invoice01Icon,
  PackageIcon,
  Settings01Icon,
  ShoppingBasket01Icon,
  ShoppingCart02Icon,
  TradeUpIcon,
  UserGroupIcon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button.tsx";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card.tsx";
import { Link } from "@tanstack/react-router";
import type { Session } from "@/routes/__root.tsx";
import PageHeader from "./page-header.tsx";

type Props = {
  session: NonNullable<Session>;
};

const DashboardPage = ({ session }: Props) => {
  const userName = session.user.name || session.user.email.split("@")[0];

  // Key features of the platform
  const features = [
    {
      title: "Product Management",
      description:
        "Register products, control inventory, organize by categories and brands",
      icon: PackageIcon,
      href: "/app/products",
      color: "bg-primary",
      highlights: [
        "Inventory control",
        "Categories and brands",
        "Variant SKUs",
      ],
    },
    {
      title: "Sales and Checkout",
      description: "Fast POS, order creation and complete sales management",
      icon: ShoppingCart02Icon,
      href: "/app/checkout",
      color: "bg-primary",
      highlights: [
        "Fast checkout",
        "Order management",
        "Multiple payment methods",
      ],
    },
    {
      title: "Customer Management",
      description:
        "Register customers, organize information and purchase history",
      icon: UserGroupIcon,
      href: "/app/customers",
      color: "bg-primary",
      highlights: ["Complete data", "Valid tax ID", "Purchase history"],
    },
    {
      title: "Reports and Analytics",
      description: "Track sales, revenue and your store's performance",
      icon: Analytics01Icon,
      href: "/app/reports",
      color: "bg-accent",
      highlights: ["Sales reports", "Performance analysis", "Real-time data"],
    },
    {
      title: "Financial Management",
      description: "Control payments, receipts and cash flow",
      icon: CreditCardIcon,
      href: "/app/finance",
      color: "bg-primary",
      highlights: ["Cash control", "Payments", "Receipts"],
    },
    {
      title: "Settings",
      description: "Customize your store and configure features",
      icon: Settings01Icon,
      href: "/app/settings",
      color: "bg-gray-500",
      highlights: ["Company data", "Integrations", "Preferences"],
    },
  ];

  // Quick actions that users can take
  const quickActions = [
    {
      title: "New Sale",
      description: "Start a quick sale",
      icon: ShoppingBasket01Icon,
      href: "/app/checkout",
      color: "bg-primary",
    },
    {
      title: "Create Order",
      description: "Detailed order",
      icon: Invoice01Icon,
      href: "/app/orders/add",
      color: "bg-primary",
    },
    {
      title: "Add Product",
      description: "Register product",
      icon: Add01Icon,
      href: "/app/products/add",
      color: "bg-primary",
    },
  ];

  return (
    <div className="w-full space-y-8 overflow-auto">
      <PageHeader
        text="Your complete management platform for small and medium businesses. Manage products, sales, customers and much more in one place."
        title={`Welcome to Strait, ${userName}! 👋`}
      />

      {/* Quick Actions */}
      <section>
        <div className="mb-6">
          <h2 className="font-semibold text-lg tracking-tight">
            Quick Actions
          </h2>
          <p className="text-muted-foreground text-sm">
            Take quick actions to get started with Strait
          </p>
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          {quickActions.map((action) => (
            <Button
              className="h-auto justify-start p-4"
              key={action.href}
              render={<Link to={action.href} />}
              variant="outline"
            >
              <div className="flex w-full items-center gap-3">
                <div className={`rounded-lg p-2 ${action.color}`}>
                  <HugeiconsIcon
                    className="h-5 w-5 text-white"
                    icon={action.icon}
                  />
                </div>
                <div className="flex-1 text-left">
                  <div className="font-medium text-sm">{action.title}</div>
                  <div className="text-muted-foreground text-xs">
                    {action.description}
                  </div>
                </div>
              </div>
            </Button>
          ))}
        </div>
      </section>

      {/* Main Features */}
      <section>
        <div className="mb-6">
          <h2 className="font-semibold text-lg tracking-tight">
            Main Features
          </h2>
          <p className="text-muted-foreground text-sm">
            Explore everything Strait offers for your business
          </p>
        </div>

        <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
          {features.map((feature) => (
            <Card
              className="group transition-shadow hover:shadow-md"
              key={feature.href}
            >
              <Link to={feature.href}>
                <CardHeader className="pb-3">
                  <div className="flex items-center gap-3">
                    <div className={`rounded-lg p-2 ${feature.color}`}>
                      <HugeiconsIcon
                        className="h-5 w-5 text-white"
                        icon={feature.icon}
                      />
                    </div>
                    <CardTitle className="text-base">{feature.title}</CardTitle>
                  </div>
                </CardHeader>
                <CardContent className="pt-0">
                  <p className="mb-3 text-muted-foreground text-sm">
                    {feature.description}
                  </p>
                  <ul className="space-y-1">
                    {feature.highlights.map((highlight) => (
                      <li
                        className="flex items-center gap-2 text-xs"
                        key={highlight}
                      >
                        <HugeiconsIcon
                          className="h-3 w-3 text-primary"
                          icon={CheckmarkCircle01Icon}
                        />
                        <span className="text-muted-foreground">
                          {highlight}
                        </span>
                      </li>
                    ))}
                  </ul>
                </CardContent>
              </Link>
            </Card>
          ))}
        </div>
      </section>

      {/* Getting Started */}
      <section>
        <Card className="border-dashed bg-muted/20">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <HugeiconsIcon className="h-5 w-5" icon={TradeUpIcon} />
              Getting Started
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <p className="text-muted-foreground text-sm">
                To start using Strait, we recommend following this sequence:
              </p>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <div className="flex items-center gap-3 rounded-lg border bg-background p-3">
                  <div className="flex h-6 w-6 items-center justify-center rounded-full bg-primary font-bold text-primary-foreground text-xs">
                    1
                  </div>
                  <div className="text-sm">
                    <div className="font-medium">Configure your company</div>
                    <div className="text-muted-foreground text-xs">
                      Basic data
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-3 rounded-lg border bg-background p-3">
                  <div className="flex h-6 w-6 items-center justify-center rounded-full bg-primary font-bold text-primary-foreground text-xs">
                    2
                  </div>
                  <div className="text-sm">
                    <div className="font-medium">Add products</div>
                    <div className="text-muted-foreground text-xs">
                      Build your catalog
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-3 rounded-lg border bg-background p-3">
                  <div className="flex h-6 w-6 items-center justify-center rounded-full bg-accent font-bold text-accent-foreground text-xs">
                    3
                  </div>
                  <div className="text-sm">
                    <div className="font-medium">Register customers</div>
                    <div className="text-muted-foreground text-xs">
                      Database
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-3 rounded-lg border bg-background p-3">
                  <div className="flex h-6 w-6 items-center justify-center rounded-full bg-primary font-bold text-primary-foreground text-xs">
                    4
                  </div>
                  <div className="text-sm">
                    <div className="font-medium">Make your first sale</div>
                    <div className="text-muted-foreground text-xs">
                      Start selling
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </section>
    </div>
  );
};

export default DashboardPage;
