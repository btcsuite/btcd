### Table of Contents
1. [Overview](#Overview)<br />
2. [Minimum Recommended Skillset](#MinSkillset)<br />
3. [Required Reading](#ReqReading)<br />
4. [Development Practices](#DevelopmentPractices)<br />
4.1. [Share Early, Share Often](#ShareEarly)<br />
4.2. [Testing](#Testing)<br />
4.3. [Code Documentation and Commenting](#CodeDocumentation)<br />
4.4. [Model Git Commit Messages](#ModelGitCommitMessages)<br />
5. [Code Approval Process](#CodeApproval)<br />
5.1 [Code Review](#CodeReview)<br />
5.2 [Rework Code (if needed)](#CodeRework)<br />
5.3 [Acceptance](#CodeAcceptance)<br />
6. [Contribution Standards](#Standards)<br />
6.1. [Contribution Checklist](#Checklist)<br />
6.2. [Licensing of Contributions](#Licensing)<br />

<a name="Overview" />

### 1. Overview

Developing cryptocurrencies is an exciting endeavor that touches a wide variety
of areas such as wire protocols, peer-to-peer networking, databases,
cryptography, language interpretation (transaction scripts), RPC, and
websockets.  They also represent a radical shift to the current fiscal system
and as a result provide an opportunity to help reshape the entire financial
system.  There are few projects that offer this level of diversity and impact
all in one code base.

However, as exciting as it is, one must keep in mind that cryptocurrencies
represent real money and introducing bugs and security vulnerabilities can have
far more dire consequences than in typical projects where having a small bug is
minimal by comparison.  In the world of cryptocurrencies, even the smallest bug
in the wrong area can cost people a significant amount of money.  For this
reason, the dcrd suite has a formalized and rigorous development process which
is outlined on this page.

We highly encourage code contributions, however it is imperative that you adhere
to the guidelines established on this page.

<a name="MinSkillset" />

### 2. Minimum Recommended Skillset

The following list is a set of core competencies that we recommend you possess
before you really start attempting to contribute code to the project.  These are
not hard requirements as we will gladly accept code contributions as long as
they follow the guidelines set forth on this page.  That said, if you don't have
the following basic qualifications you will likely find it quite difficult to
contribute.

- A reasonable understanding of bitcoin at a high level (see the
  [Required Reading](#ReqReading) section for the original white paper)
- Experience in some type of C-like language
- An understanding of data structures and their performance implications
- Familiarity with unit testing
- Debugging experience
- Ability to understand not only the area you are making a change in, but also
  the code your change relies on, and the code which relies on your changed code

Building on top of those core competencies, the recommended skill set largely
depends on the specific areas you are looking to contribute to.  For example,
if you wish to contribute to the cryptography code, you should have a good
understanding of the various aspects involved with cryptography such as the
security and performance implications.

<a name="ReqReading" />

### 3. Required Reading

- [Effective Go](http://golang.org/doc/effective_go.html) - The entire dcrd
  suite follows the guidelines in this document.  For your code to be accepted,
  it must follow the guidelines therein.
- [Original Satoshi Whitepaper](http://www.google.com/url?sa=t&rct=j&q=&esrc=s&source=web&cd=1&cad=rja&ved=0CCkQFjAA&url=http%3A%2F%2Fbitcoin.org%2Fbitcoin.pdf&ei=os3VUuH8G4SlsASV74GoAg&usg=AFQjCNEipPLigou_1MfB7DQjXCNdlylrBg&sig2=FaHDuT5z36GMWDEnybDJLg&bvm=bv.59378465,d.b2I) - This is the white paper that started it all.  Having a solid
  foundation to build on will make the code much more comprehensible.

<a name="DevelopmentPractices" />

### 4. Development Practices

Developers are expected to work in their own trees and submit pull requests when
they feel their feature or bug fix is ready for integration into the  master
branch.

<a name="ShareEarly" />

### 4.1 Share Early, Share Often

We firmly believe in the share early, share often approach.  The basic premise
of the approach is to announce your plans **before** you start work, and once
you have started working, craft your changes into a stream of small and easily
reviewable commits.

This approach has several benefits:

- Announcing your plans to work on a feature **before** you begin work avoids
  duplicate work
- It permits discussions which can help you achieve your goals in a way that is
  consistent with the existing architecture
- It minimizes the chances of you spending time and energy on a change that
  might not fit with the consensus of the community or existing architecture and
  potentially be rejected as a result
- Incremental development helps ensure you are on the right track with regards
  to the rest of the community
- The quicker your changes are merged to master, the less time you will need to
  spend rebasing and otherwise trying to keep up with the main code base

<a name="Testing" />

### 4.2 Testing

One of the major design goals of all core dcrd packages is to aim for complete
test coverage.  This is financial software so bugs and regressions can cost
people real money.  For this reason every effort must be taken to ensure the
code is as accurate and bug-free as possible.  Thorough testing is a good way to
help achieve that goal.

Unless a new feature you submit is completely trivial, it will probably be
rejected unless it is also accompanied by adequate test coverage for both
positive and negative conditions.  That is to say, the tests must ensure your
code works correctly when it is fed correct data as well as incorrect data
(error paths).

Go provides an excellent test framework that makes writing test code and
checking coverage statistics straight forward.  For more information about the
test coverage tools, see the [golang cover blog post](http://blog.golang.org/cover).

A quick summary of test practices follows:
- All new code should be accompanied by tests that ensure the code behaves
  correctly when given expected values, and, perhaps even more importantly, that
  it handles errors gracefully
- When you fix a bug, it should be accompanied by tests which exercise the bug
  to both prove it has been resolved and to prevent future regressions

<a name="CodeDocumentation" />

### 4.3 Code Documentation and Commenting

- At a minimum every function must be commented with its intended purpose and
  any assumptions that it makes
  - Function comments must always begin with the name of the function per
    [Effective Go](http://golang.org/doc/effective_go.html)
  - Function comments should be complete sentences since they allow a wide
    variety of automated presentations such as [godoc.org](https://godoc.org)
  - The general rule of thumb is to look at it as if you were completely
    unfamiliar with the code and ask yourself, would this give me enough
	information to understand what this function does and how I'd probably want
	to use it?
- Exported functions should also include detailed information the caller of the
  function will likely need to know and/or understand:<br /><br />
**WRONG**
```Go
// convert a compact uint32 to big.Int
func CompactToBig(compact uint32) *big.Int {
```
**RIGHT**
```Go
// CompactToBig converts a compact representation of a whole number N to a
// big integer.  The representation is similar to IEEE754 floating point
// numbers.
//
// Like IEEE754 floating point, there are three basic components: the sign,
// the exponent, and the mantissa. They are broken out as follows:
//
//        * the most significant 8 bits represent the unsigned base 256 exponent
//        * bit 23 (the 24th bit) represents the sign bit
//        * the least significant 23 bits represent the mantissa
//
//        -------------------------------------------------
//        |   Exponent     |    Sign    |    Mantissa     |
//        -------------------------------------------------
//        | 8 bits [31-24] | 1 bit [23] | 23 bits [22-00] |
//        -------------------------------------------------
//
// The formula to calculate N is:
//         N = (-1^sign) * mantissa * 256^(exponent-3)
//
// This compact form is only used in bitcoin to encode unsigned 256-bit numbers
// which represent difficulty targets, thus there really is not a need for a
// sign bit, but it is implemented here to stay consistent with bitcoind.
func CompactToBig(compact uint32) *big.Int {
```
- Comments in the body of the code are highly encouraged, but they should
  explain the intention of the code as opposed to just calling out the
  obvious<br /><br />
**WRONG**
```Go
// return err if amt is less than 5460
if amt < 5460 {
	return err
}
```
**RIGHT**
```Go
// Treat transactions with amounts less than the amount which is considered dust
// as non-standard.
if amt < 5460 {
	return err
}
```
**NOTE:** The above should really use a constant as opposed to a magic number,
but it was left as a magic number to show how much of a difference a good
comment can make.

<a name="ModelGitCommitMessages" />

### 4.4 Model Git Commit Messages

This project prefers to keep a clean commit history with well-formed commit
messages.  This section illustrates a model commit message and provides a bit
of background for it.  This content was originally created by Tim Pope and made
available on his website, however that website is no longer active, so it is
being provided here.

Here’s a model Git commit message:

```
prefix: Short summary of changes (max 50 chars)

More detailed explanatory text, if necessary.  Wrap it to about 72
characters or so.  In some contexts, the first line is treated as the
subject of an email and the rest of the text as the body.  The blank
line separating the summary from the body is critical (unless you omit
the body entirely); tools like rebase can get confused if you run the
two together.

Write your commit message in the present tense: "Fix bug" and not "Fixed
bug."  This convention matches up with commit messages generated by
commands like git merge and git revert.

Further paragraphs come after blank lines.

- Bullet points are okay, too
- Typically a hyphen or asterisk is used for the bullet, preceded by a
  single space, with blank lines in between, but conventions vary here
- Use a hanging indent
```

The commit prefix is always of the form `prefix: `. It is for the sole  
purpose of indicating which package or component was touched in a  
commit.  

Here is how the right prefix for a commit is chosen.
- If a commit modifies a component in the main package  
  (eg. `blocklogger`) use the component name as the commit prefix.
- If a commit modifies a component in any of the packages besides the main  
  package use the package name as the commit prefix (eg. `dcrjson`).
- If a commit modifies components in multiple packages use the word `multi`  
  as the commit prefix.

Here are some of the reasons why wrapping your commit messages to 72 columns is
a good thing.

- git log doesn’t do any special special wrapping of the commit messages. With
  the default pager of less -S, this means your paragraphs flow far off the edge
  of the screen, making them difficult to read. On an 80 column terminal, if we
  subtract 4 columns for the indent on the left and 4 more for symmetry on the
  right, we’re left with 72 columns.
- git format-patch --stdout converts a series of commits to a series of emails,
  using the messages for the message body.  Good email netiquette dictates we
  wrap our plain text emails such that there’s room for a few levels of nested
  reply indicators without overflow in an 80 column terminal.

<a name="CodeApproval" />

### 5. Code Approval Process

This section describes the code approval process that is used for code
contributions.  This is how to get your changes into dcrd.

<a name="CodeReview" />

### 5.1 Code Review

All code which is submitted will need to be reviewed before inclusion into the
master branch.  This process is performed by the project maintainers and usually
other committers who are interested in the area you are working in as well.

##### Code Review Timeframe

The timeframe for a code review will vary greatly depending on factors such as
the number of other pull requests which need to be reviewed, the size and
complexity of the contribution, how well you followed the guidelines presented
on this page, and how easy it is for the reviewers to digest your commits.  For
example, if you make one monolithic commit that makes sweeping changes to things
in multiple subsystems, it will obviously take much longer to review.  You will
also likely be asked to split the commit into several smaller, and hence more
manageable, commits.

Keeping the above in mind, most small changes will be reviewed within a few
days, while large or far reaching changes may take weeks.  This is a good reason
to stick with the [Share Early, Share Often](#ShareOften) development practice
outlined above.

##### What is the review looking for?

The review is mainly ensuring the code follows the [Development Practices](#DevelopmentPractices)
and [Code Contribution Standards](#Standards).  However, there are a few other
checks which are generally performed as follows:

- The code is stable and has no stability or security concerns
- The code is properly using existing APIs and generally fits well into the
  overall architecture
- The change is not something which is deemed inappropriate by community
  consensus

<a name="CodeRework" />

### 5.2 Rework Code (if needed)

After the code review, the change will be accepted immediately if no issues are
found.  If there are any concerns or questions, you will be provided with
feedback along with the next steps needed to get your contribution merged with
master.  In certain cases the code reviewer(s) or interested committers may help
you rework the code, but generally you will simply be given feedback for you to
make the necessary changes.

This process will continue until the code is finally accepted.

<a name="CodeAcceptance" />

### 5.3 Acceptance

Once your code is accepted, it will be integrated with the master branch.
Typically it will be rebased and fast-forward merged to master as we prefer to
keep a clean commit history over a tangled weave of merge commits.  However,
regardless of the specific merge method used, the code will be integrated with
the master branch and the pull request will be closed.

Rejoice as you will now be listed as a [contributor](https://github.com/decred/dcrd/graphs/contributors)!

<a name="Standards" />

### 6. Contribution Standards

<a name="Checklist" />

### 6.1. Contribution Checklist

- [&nbsp;&nbsp;] All changes are Go version 1.3 compliant
- [&nbsp;&nbsp;] The code being submitted is commented according to the
  [Code Documentation and Commenting](#CodeDocumentation) section
- [&nbsp;&nbsp;] For new code: Code is accompanied by tests which exercise both
  the positive and negative (error paths) conditions (if applicable)
- [&nbsp;&nbsp;] For bug fixes: Code is accompanied by new tests which trigger
  the bug being fixed to prevent regressions
- [&nbsp;&nbsp;] Any new logging statements use an appropriate subsystem and
  logging level
- [&nbsp;&nbsp;] Code has been formatted with `go fmt`
- [&nbsp;&nbsp;] Running `go test` does not fail any tests
- [&nbsp;&nbsp;] Running `go vet` does not report any issues
- [&nbsp;&nbsp;] Running [golint](https://github.com/golang/lint) does not
  report any **new** issues that did not already exist

<a name="Licensing" />

### 6.2. Licensing of Contributions
****
All contributions must be licensed with the
[ISC license](https://github.com/decred/dcrd/blob/master/LICENSE).  This is
the same license as all of the code in the dcrd suite.
