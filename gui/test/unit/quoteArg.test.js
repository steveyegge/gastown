/**
 * Gas Town GUI - quoteArg Unit Tests
 *
 * Tests for the shell argument quoting function in server.js
 * Critical for preventing command injection attacks
 */

import { describe, it, expect } from 'vitest';

// Copy of quoteArg from server.js for testing
// (In production, this would be imported from a shared module)
function quoteArg(arg) {
  if (arg === null || arg === undefined) return "''";
  const str = String(arg);
  // Single quotes are the safest - only need to escape single quotes themselves
  // Replace each ' with '\'' (end quote, escaped quote, start quote)
  return "'" + str.replace(/'/g, "'\\''") + "'";
}

describe('quoteArg', () => {
  describe('basic functionality', () => {
    it('should wrap simple strings in single quotes', () => {
      expect(quoteArg('hello')).toBe("'hello'");
    });

    it('should handle empty strings', () => {
      expect(quoteArg('')).toBe("''");
    });

    it('should handle null', () => {
      expect(quoteArg(null)).toBe("''");
    });

    it('should handle undefined', () => {
      expect(quoteArg(undefined)).toBe("''");
    });

    it('should convert numbers to strings', () => {
      expect(quoteArg(123)).toBe("'123'");
    });

    it('should handle strings with spaces', () => {
      expect(quoteArg('hello world')).toBe("'hello world'");
    });
  });

  describe('special character escaping', () => {
    it('should escape single quotes', () => {
      expect(quoteArg("it's")).toBe("'it'\\''s'");
    });

    it('should escape multiple single quotes', () => {
      expect(quoteArg("'hello' 'world'")).toBe("''\\''hello'\\'' '\\''world'\\'''");
    });

    it('should NOT escape double quotes (safe inside single quotes)', () => {
      expect(quoteArg('hello "world"')).toBe("'hello \"world\"'");
    });
  });

  describe('command injection prevention', () => {
    it('should safely handle backticks', () => {
      const result = quoteArg('`rm -rf /`');
      expect(result).toBe("'`rm -rf /`'");
      // Inside single quotes, backticks are literal, not executed
    });

    it('should safely handle $() subshell', () => {
      const result = quoteArg('$(rm -rf /)');
      expect(result).toBe("'$(rm -rf /)'");
      // Inside single quotes, $() is literal, not executed
    });

    it('should safely handle ${} variable expansion', () => {
      const result = quoteArg('${PATH}');
      expect(result).toBe("'${PATH}'");
      // Inside single quotes, ${} is literal, not expanded
    });

    it('should safely handle semicolons', () => {
      const result = quoteArg('foo; rm -rf /');
      expect(result).toBe("'foo; rm -rf /'");
    });

    it('should safely handle pipes', () => {
      const result = quoteArg('foo | cat /etc/passwd');
      expect(result).toBe("'foo | cat /etc/passwd'");
    });

    it('should safely handle redirects', () => {
      const result = quoteArg('foo > /etc/passwd');
      expect(result).toBe("'foo > /etc/passwd'");
    });

    it('should safely handle newlines', () => {
      const result = quoteArg('foo\nrm -rf /');
      expect(result).toBe("'foo\nrm -rf /'");
    });

    it('should safely handle ampersands', () => {
      const result = quoteArg('foo && rm -rf /');
      expect(result).toBe("'foo && rm -rf /'");
    });

    it('should safely handle logical OR', () => {
      const result = quoteArg('foo || rm -rf /');
      expect(result).toBe("'foo || rm -rf /'");
    });

    it('should safely handle complex injection attempt', () => {
      const malicious = "'; rm -rf /; echo '";
      const result = quoteArg(malicious);
      // This should be safely quoted - single quotes escaped with '\''
      // Input: '; rm -rf /; echo '
      // Each ' becomes '\'' so result is: ''\''; rm -rf /; echo '\'''
      expect(result).toBe("''\\''; rm -rf /; echo '\\'''");
    });
  });

  describe('realistic rig/agent names', () => {
    it('should handle typical rig names', () => {
      expect(quoteArg('hytopia-map-compression')).toBe("'hytopia-map-compression'");
    });

    it('should handle rig names with dots', () => {
      expect(quoteArg('repo.v2.beta')).toBe("'repo.v2.beta'");
    });

    it('should handle session names', () => {
      expect(quoteArg('gt-myproject-capable')).toBe("'gt-myproject-capable'");
    });

    it('should handle paths', () => {
      expect(quoteArg('/home/user/gt/rig/config.json')).toBe("'/home/user/gt/rig/config.json'");
    });

    it('should handle paths with spaces', () => {
      expect(quoteArg('/home/user/My Projects/rig')).toBe("'/home/user/My Projects/rig'");
    });
  });
});
