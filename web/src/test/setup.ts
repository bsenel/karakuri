import '@testing-library/jest-dom/vitest';
import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';

// React Testing Library leaves the previous render mounted otherwise, so a
// query for "the Users link" would find one from a test that already finished.
afterEach(cleanup);
