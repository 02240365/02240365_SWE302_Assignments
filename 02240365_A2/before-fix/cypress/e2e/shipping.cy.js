// shipping.cy.js - Shipping Service: quotes by zone, free shipping, tracking.

describe('Shipping and tracking', () => {
  beforeEach(() => {
    cy.openStore('/orders.html');
  });

  // ---- VALID ----
  it('quotes each customer by delivery zone', () => {
    cy.fixture('customers').then((customers) => {
      ['c001', 'c002', 'c003'].forEach((key) => {
        const c = customers[key];
        cy.api('GET', `/api/shipping/quote?customerId=${c.id}&subtotal=1000&itemCount=1`).then((res) => {
          expect(res.status, c.id).to.eq(200);
          expect(res.body.zone).to.eq(c.zone);
          expect(res.body.fee).to.eq(c.fee);
          expect(res.body.etaDays).to.eq(c.eta);
          expect(res.body.address).to.eq(c.address);
        });
      });
    });
  });

  it('makes shipping free at exactly Nu. 5,000 but not at 4,999.99', () => {
    cy.api('GET', '/api/shipping/quote?customerId=C001&subtotal=5000&itemCount=1').its('body.freeShipping').should('eq', true);
    cy.api('GET', '/api/shipping/quote?customerId=C001&subtotal=4999.99&itemCount=1').then((res) => {
      expect(res.body.freeShipping).to.eq(false);
      expect(res.body.fee).to.eq(100);
    });
  });

  it('tracks a parcel after an order', () => {
    cy.api('POST', '/api/orders', { customerId: 'C002', productId: 'P003', quantity: 1 }).then((order) => {
      const tracking = order.body.trackingId;
      cy.enterCustomer('C002');
      cy.get('[data-cy=track-input]').type(tracking);
      cy.get('[data-cy=track-btn]').click();
      cy.get('[data-cy=track-status]').should('contain', tracking).and('contain', 'PROCESSING');
      cy.get('[data-cy=track-result]').should('contain', order.body.orderId).and('contain', 'Phuentsholing');
      cy.get('[data-cy=track-event]').should('have.length', 1);
    });
  });

  it('finds a parcel whatever the letter case of the tracking ID', () => {
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P003', quantity: 1 }).then((order) => {
      cy.api('GET', `/api/shipping/track/${order.body.trackingId.toLowerCase()}`).its('status').should('eq', 200);
    });
  });

  // ---- INVALID ----
  it('says when the tracking ID is unknown', () => {
    cy.get('[data-cy=track-input]').type('TRK-99999');
    cy.get('[data-cy=track-btn]').click();
    cy.get('[data-cy=track-error]').should('contain', 'no shipment found');
  });

  it('refuses a quote with missing or bad values (API)', () => {
    cy.api('GET', '/api/shipping/quote?customerId=C001&subtotal=abc&itemCount=1').its('status').should('eq', 400);
    cy.api('GET', '/api/shipping/quote?subtotal=100&itemCount=1').its('status').should('eq', 400);
    cy.api('GET', '/api/shipping/quote?customerId=C001&subtotal=0&itemCount=1').its('status').should('eq', 400);
    cy.api('GET', '/api/shipping/quote?customerId=C001&subtotal=100&itemCount=0').its('status').should('eq', 400);
    cy.api('GET', '/api/shipping/quote?customerId=C999&subtotal=100&itemCount=1').its('status').should('eq', 404);
  });
});
