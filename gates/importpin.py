#!/usr/bin/env python3
"""Import-pin: structural independence for the naive fold.

The naive fold in fixtures/generate.py may import only Python stdlib names
from the allowlist below and may not reference the Go tree. This makes the
independence claim STRUCTURAL: the Python and Go folds do not share code.

It does NOT make the claim epistemic: both folds were written from the same
written contract, so a contract-level misunderstanding reproduces on both sides,
and this gate stays green. That distinction is binding: the gate must support
exactly the claim it enforces.

A static allowlist cannot defend against a determined author—arbitrary Python
obfuscates arbitrarily. What this gate buys is protection against drift and
accident: a future edit that quietly adds a dependency, a copied helper that
pulls in a module, a path string that creeps in. It catches the silent changes
that break the independence claim.

--self-test injects forbidden patterns into temp copies and requires the check
to FAIL on them (a gate that cannot go red proves nothing).
"""
import ast
import os
import sys
import tempfile

ALLOWED = {"argparse", "datetime", "hashlib", "json", "os", "random", "sys"}
FORBIDDEN_LITERALS = ("internal/", "cmd/", ".go", "go run", "go build")
FORBIDDEN_NAMES = {"exec", "eval", "__import__", "compile"}
DEFAULT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "fixtures", "generate.py")


def fold_string_constants(node):
    """Fold string constants, including formatted strings.

    Handles:
    - Concatenation: "a" + "b"
    - % formatting with all-literal args: "%s" % "a" or "%s %s" % ("a", "b")
    - .format() with all-literal args: "{}".format("a")

    Returns (is_string, value) if the node evaluates to a string at parse time,
    else (False, None).

    When operands include variables or complex expressions, cannot fold and
    returns (False, None) — static analysis cannot determine the value.
    """
    if isinstance(node, ast.Constant) and isinstance(node.value, str):
        return (True, node.value)

    # Handle binary operations (+ and %)
    if isinstance(node, ast.BinOp):
        if isinstance(node.op, ast.Add):
            left_is_str, left_val = fold_string_constants(node.left)
            right_is_str, right_val = fold_string_constants(node.right)
            if left_is_str and right_is_str and left_val is not None and right_val is not None:
                return (True, left_val + right_val)

        elif isinstance(node.op, ast.Mod):
            # Handle % formatting: "fmt" % args
            left_is_str, fmt = fold_string_constants(node.left)
            if not (left_is_str and fmt is not None):
                return (False, None)

            # Right side can be a single value or a tuple
            right_is_str, right_val = fold_string_constants(node.right)
            if right_is_str and right_val is not None:
                # Single value
                try:
                    return (True, fmt % right_val)
                except (TypeError, ValueError):
                    return (False, None)
            elif isinstance(node.right, ast.Tuple):
                # Multiple values
                args = []
                for elt in node.right.elts:
                    is_const, val = fold_string_constants(elt)
                    if not (is_const and val is not None):
                        return (False, None)
                    args.append(val)
                try:
                    return (True, fmt % tuple(args))
                except (TypeError, ValueError):
                    return (False, None)

    # Handle .format() method calls
    if isinstance(node, ast.Call):
        if isinstance(node.func, ast.Attribute) and node.func.attr == "format":
            # Check if the string being formatted is a constant
            string_is_const, string_val = fold_string_constants(node.func.value)
            if not (string_is_const and string_val is not None):
                return (False, None)

            # Collect positional arguments
            args = []
            for arg in node.args:
                is_const, val = fold_string_constants(arg)
                if not (is_const and val is not None):
                    return (False, None)
                args.append(val)

            # Collect keyword arguments
            kwargs = {}
            for keyword in node.keywords:
                is_const, val = fold_string_constants(keyword.value)
                if not (is_const and val is not None):
                    return (False, None)
                kwargs[keyword.arg] = val

            try:
                return (True, string_val.format(*args, **kwargs))
            except (IndexError, KeyError, ValueError):
                return (False, None)

    return (False, None)


def check(path):
    try:
        with open(path, encoding="ascii") as f:
            src = f.read()
    except FileNotFoundError:
        return "file not found: %s" % path
    except UnicodeDecodeError:
        return "file is not ASCII: %s" % path

    try:
        tree = ast.parse(src, filename=path)
    except SyntaxError as e:
        return "syntax error: %s" % e.msg

    for node in ast.walk(tree):
        # Check for bare references to dangerous names (not just calls)
        if isinstance(node, ast.Name) and node.id in FORBIDDEN_NAMES:
            return "bare reference to forbidden name: %s" % node.id

        # Check for attribute access on __builtins__
        if isinstance(node, ast.Attribute):
            if isinstance(node.value, ast.Name) and node.value.id == "__builtins__":
                return "attribute access on __builtins__ is forbidden"

        # Check for getattr on __builtins__
        if isinstance(node, ast.Call):
            if isinstance(node.func, ast.Name) and node.func.id == "getattr":
                if (len(node.args) > 0 and
                    isinstance(node.args[0], ast.Name) and
                    node.args[0].id == "__builtins__"):
                    return "getattr on __builtins__ is forbidden"

        # Check imports
        if isinstance(node, ast.Import):
            for a in node.names:
                root = a.name.split(".")[0]
                if root not in ALLOWED:
                    return "import %s not in allowlist" % a.name
        elif isinstance(node, ast.ImportFrom):
            root = (node.module or "").split(".")[0]
            if root not in ALLOWED:
                return "from %s import ... not in allowlist" % node.module

        # Check string literals, including folded BinOps
        is_str, value = fold_string_constants(node)
        if is_str and value is not None:
            for lit in FORBIDDEN_LITERALS:
                if lit in value:
                    return "string literal references the Go tree: %r" % value

    return None


def main(argv):
    if argv and argv[0] == "--self-test":
        try:
            with open(DEFAULT, encoding="ascii") as f:
                src = f.read()
        except Exception as e:
            print("FAIL import-pin self-test: cannot read default file: %s" % e)
            return 1

        test_cases = [
            ("import internal.fold", "import internal.fold not in allowlist"),
            ("_e = exec", "bare reference to forbidden name: exec"),
            ("f = getattr(__builtins__, 'eval')", "getattr on __builtins__ is forbidden"),
            ('x = "int" + "ernal/"', "string literal references the Go tree"),
            ('x = "%s/" % "internal"', "string literal references the Go tree"),
            ('x = "{}/".format("internal")', "string literal references the Go tree"),
        ]

        failed_tests = []
        for poison, expected_substring in test_cases:
            with tempfile.NamedTemporaryFile("w", suffix=".py", delete=False, encoding="ascii") as tmp:
                tmp.write(poison + "\n" + src)
                name = tmp.name
            try:
                reason = check(name)
            finally:
                os.unlink(name)

            if reason is None:
                failed_tests.append("not caught: %s" % poison[:50])
            elif expected_substring not in reason:
                failed_tests.append("expected substring %r not in reason %r" % (expected_substring, reason))

        # Test failure modes
        # 1. Missing file
        reason = check("/nonexistent/file.py")
        if reason is None or "file not found" not in reason:
            failed_tests.append("missing file not caught")

        # 2. Syntax error
        with tempfile.NamedTemporaryFile("w", suffix=".py", delete=False, encoding="ascii") as tmp:
            tmp.write("def ( ):")
            name = tmp.name
        try:
            reason = check(name)
            if reason is None or "syntax error" not in reason:
                failed_tests.append("syntax error not caught")
        finally:
            os.unlink(name)

        # 3. Non-ASCII file
        with tempfile.NamedTemporaryFile("wb", suffix=".py", delete=False) as tmp:
            tmp.write(b"# \xe9\n")
            name = tmp.name
        try:
            reason = check(name)
            if reason is None or "not ASCII" not in reason:
                failed_tests.append("non-ASCII file not caught")
        finally:
            os.unlink(name)

        if failed_tests:
            for failure in failed_tests:
                print("FAIL import-pin self-test: %s" % failure)
            return 1

        print("ok import-pin self-test (all negative controls caught)")
        return 0

    path = argv[0] if argv else DEFAULT
    reason = check(path)
    if reason:
        print("FAIL import-pin: " + reason)
        return 1
    print("ok import-pin " + os.path.relpath(path))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
