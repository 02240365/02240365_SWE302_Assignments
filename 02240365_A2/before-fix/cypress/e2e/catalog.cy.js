// catalog.cy.js - product catalogue and "Find a Product by ID" (Product Service).
// Every test starts from a reset store and a fresh page, so none depends on another.

describe('Catalogue and product lookup', () => {
  beforeEach(() => {
    cy.openStore();
  });

  // ---- VALID ----
  it('shows every product with its name and price', () => {
    cy.fixture('products').then(({ catalogue }) => {
      cy.get('[data-cy=product-grid] [data-cy^=product-card-]').should('have.length', catalogue.length);
      catalogue.forEach((p) => {
        cy.card(p.id).should('contain', p.name).and('contain', `Nu. ${p.price.toFixed(2)}`);
      });
    });
  });

  it('finds a product by its ID', () => {
    cy.get('[data-cy=lookup-input]').type('P002');
    cy.get('[data-cy=lookup-btn]').click();
    cy.get('[data-cy=lookup-result]').should('contain', 'Mechanical Keyboard').and('contain', 'Nu. 4500.00');
  });

  it('ignores spaces around the product ID', () => {
    cy.get('[data-cy=lookup-input]').type('  P003  ');
    cy.get('[data-cy=lookup-btn]').click();
    cy.get('[data-cy=lookup-result]').should('contain', 'Wireless Mouse');
  });

  // ---- INVALID ----
  it('says clearly when the product does not exist', () => {
    cy.fixture('products').then(({ unknownId }) => {
      cy.get('[data-cy=lookup-input]').type(unknownId);
      cy.get('[data-cy=lookup-btn]').click();
      cy.get('[data-cy=lookup-result]').should('have.class', 'state-warning').and('contain', `Product '${unknownId}' does not exist.`);
    });
  });

  it('asks for an ID when the box is empty', () => {
    cy.get('[data-cy=lookup-btn]').click();
    cy.get('[data-cy=lookup-result]').should('contain', 'Please enter a product ID.');
  });

  it('treats an SQL-looking ID as plain text and finds nothing', () => {
    cy.get('[data-cy=lookup-input]').type("P001' OR '1'='1");
    cy.get('[data-cy=lookup-btn]').click();
    cy.get('[data-cy=lookup-result]').should('contain', 'does not exist');
  });

  it('shows HTML typed into the ID box as text, not as markup', () => {
    cy.get('[data-cy=lookup-input]').type('<b id="injected">x</b>', { parseSpecialCharSequences: false });
    cy.get('[data-cy=lookup-btn]').click();
    cy.get('[data-cy=lookup-result]').should('contain', 'does not exist');
    cy.get('#injected').should('not.exist');
  });
});
