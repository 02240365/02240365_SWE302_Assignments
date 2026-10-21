// AWS Lambda: calculateOrderDiscount (Node.js runtime, handler: index.handler)
// Tiers by quantity: >=2 -> 5%, >=3 -> 10%, >=5 -> 15%.
// NOTE: reconstructed from the Lab 5 behaviour; compare with the version
// deployed in your AWS console and keep whichever you prefer.

const round2 = (n) => Math.round((n + Number.EPSILON) * 100) / 100;

export const handler = async (event) => {
  const { productId, unitPrice, quantity } = event || {};

  if (
    typeof productId !== "string" || productId === "" ||
    typeof unitPrice !== "number" || !(unitPrice > 0) ||
    !Number.isInteger(quantity) || quantity <= 0
  ) {
    return {
      success: false,
      errorCode: "invalid_input",
      message: "productId, a positive unitPrice and a positive integer quantity are required.",
    };
  }

  let discountPercent = 0;
  if (quantity >= 5) discountPercent = 15;
  else if (quantity >= 3) discountPercent = 10;
  else if (quantity >= 2) discountPercent = 5;

  const subtotal = round2(unitPrice * quantity);
  const discountAmount = round2((subtotal * discountPercent) / 100);
  const finalTotal = round2(subtotal - discountAmount);

  return {
    success: true,
    productId,
    quantity,
    unitPrice,
    subtotal,
    discountPercent,
    discountAmount,
    finalTotal,
  };
};
