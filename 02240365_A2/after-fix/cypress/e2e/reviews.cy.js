// reviews.cy.js - Review Service (rating 1-5, comment 1-200 characters,
// one review per customer per product, average rating).

describe('Product reviews', () => {
  beforeEach(() => {
    cy.openStore('/reviews.html');
  });

  const writeReview = (rating, comment) => {
    cy.get('[data-cy=review-rating]').select(String(rating));
    if (comment !== '') cy.get('[data-cy=review-comment]').type(comment, { parseSpecialCharSequences: false });
    cy.get('[data-cy=review-submit]').click();
  };

  // ---- VALID ----
  it('starts with no reviews', () => {
    cy.get('[data-cy=review-average]').should('contain', 'No reviews yet');
  });

  it('adds a review and shows it with the average', () => {
    cy.fixture('reviews').then(({ good }) => {
      cy.enterCustomer('C001');
      cy.get('[data-cy=review-product]').select('P003');
      writeReview(good.rating, good.comment);
      cy.get('[data-cy=review-ok]').should('contain', 'Thank you');
      cy.get('[data-cy=review-item]').should('have.length', 1).and('contain', good.comment).and('contain', 'Sonam Zangmo');
      cy.get('[data-cy=review-average]').should('contain', '5.0');
    });
  });

  it('averages the ratings of several customers', () => {
    cy.api('POST', '/api/reviews', { productId: 'P001', customerId: 'C001', rating: 5, comment: 'Great' });
    cy.api('POST', '/api/reviews', { productId: 'P001', customerId: 'C002', rating: 4, comment: 'Good' });
    cy.get('[data-cy=review-load-btn]').click();
    cy.get('[data-cy=review-average]').should('contain', '4.5');
    cy.get('[data-cy=review-count]').should('contain', '2 reviews');
    cy.get('[data-cy=review-item]').first().should('contain', 'Good'); // newest first
  });

  it('accepts a comment of exactly 200 characters', () => {
    cy.api('POST', '/api/reviews', { productId: 'P001', customerId: 'C001', rating: 3, comment: 'a'.repeat(200) })
      .its('status').should('eq', 201);
  });

  it('lets the same customer review a different product', () => {
    cy.api('POST', '/api/reviews', { productId: 'P001', customerId: 'C001', rating: 3, comment: 'one' });
    cy.api('POST', '/api/reviews', { productId: 'P002', customerId: 'C001', rating: 3, comment: 'two' })
      .its('status').should('eq', 201);
  });

  // ---- INVALID ----
  it('asks for a Customer ID before reviewing', () => {
    writeReview(5, 'Nice');
    cy.get('[data-cy=review-error]').should('contain', 'Please enter your Customer ID');
  });

  it('asks for a comment', () => {
    cy.enterCustomer('C001');
    writeReview(5, '');
    cy.get('[data-cy=review-error]').should('contain', 'please write a comment');
  });

  it('refuses a comment that is only spaces', () => {
    cy.api('POST', '/api/reviews', { productId: 'P001', customerId: 'C001', rating: 5, comment: '     ' })
      .its('status').should('eq', 400);
  });

  it('refuses a comment longer than 200 characters', () => {
    cy.api('POST', '/api/reviews', { productId: 'P001', customerId: 'C001', rating: 5, comment: 'a'.repeat(201) }).then((res) => {
      expect(res.status).to.eq(400);
      expect(res.body.message).to.contain('too long');
    });
  });

  it('refuses ratings outside 1 to 5', () => {
    [0, 6, -1].forEach((rating) => {
      cy.api('POST', '/api/reviews', { productId: 'P001', customerId: 'C001', rating, comment: 'x' })
        .its('status').should('eq', 400);
    });
  });

  it('allows only one review per customer per product', () => {
    cy.enterCustomer('C001');
    writeReview(5, 'First');
    cy.get('[data-cy=review-ok]');
    writeReview(1, 'Second');
    cy.get('[data-cy=review-error]').should('contain', 'already reviewed');
    cy.get('[data-cy=review-item]').should('have.length', 1);
  });

  it('refuses an unknown product or customer (API)', () => {
    cy.api('POST', '/api/reviews', { productId: 'P999', customerId: 'C001', rating: 5, comment: 'x' }).its('status').should('eq', 404);
    cy.api('POST', '/api/reviews', { productId: 'P001', customerId: 'C999', rating: 5, comment: 'x' }).its('status').should('eq', 404);
    cy.api('GET', '/api/reviews/P999').its('status').should('eq', 404);
  });

  // ---- SECURITY ----
  it('shows HTML in a review as plain text (no script, no markup)', () => {
    cy.fixture('reviews').then(({ xss }) => {
      cy.enterCustomer('C001');
      writeReview(5, xss);
      cy.get('[data-cy=review-text]').should('contain', '<b>bold</b>'); // shown literally
      cy.get('[data-cy=review-list] img').should('not.exist');
      cy.get('[data-cy=review-list] b').should('not.exist');
      cy.window().then((win) => expect(win.__xss, 'injected script ran').to.be.undefined);
    });
  });
});
