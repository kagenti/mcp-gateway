Use the test cases file as context: $ARGUMENTS

If no file path was provided above, default to: tests/e2e/test_cases.md

Ensure the test cases are covered. Add new e2e tests if the case is not covered. You should check existing test cases are fully covered. Don't modify existing test cases without prompting for permission. New test cases can be added to happy_path_test.go if the test starts with a [Happy] tag. If you find another value other than [Happy] this should be added to a new set of specs grouped under that tag value. If you find a test case without a tag such as [Happy] warn via outputting the test case. You should follow the patterns of the existing tests. Helper code should be added to the tests/e2e/commons.go file. Skip test cases already covered.
