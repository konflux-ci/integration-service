from gitlint.rules import CommitRule, RuleViolation


class ConventionalCommitsDescription(CommitRule):
    """This rule will enforce that each commit description starts with
    a lowercase."""

    id = "UC1"
    name = "contrib-description-conventional-commits"

    def validate(self, commit):
        """Validate the given commit checking that the description starts with
        a lowercase."""
        if ":" not in commit.message.title:
            self.log.debug("Title does not follow conventional commit specification")
            return

        description = commit.message.title.split(":")[1].strip()

        if len(description) > 0 and description[0].isupper():
            return [
                RuleViolation(
                    self.id, "Description should start with lowercase", line_nr=1
                )
            ]
