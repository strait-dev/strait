// "Machine" prefix avoids name collision with Convex Doc types (OrderItem, OrderPayment)

export type MachineOrderItem = {
  _id?: string;
  order_id?: string;
  sku_id: string;
  quantity: number;
  returned_quantity: number;
  price: string;
  discount: string;
  discount_type: "percentage" | "fixed" | "none";
  note: string;
  product_id: string;
  user_id: string;
  organization_id: string;
  created_at: number;
  updated_at: number;
  deleted_at: number | null;
};

export type MachineOrderPayment = {
  _id?: string;
  order_id?: string;
  amount: string;
  payment_method:
    | "cash"
    | "credit_card"
    | "pix"
    | "debit_card"
    | "store_credit";
  installments: number;
  amount_received?: string | null;
  change_amount?: string | null;
  organization_id: string;
  user_id: string;
  updated_at: number;
  created_at: number;
  deleted_at: number | null;
};
