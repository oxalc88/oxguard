from lib.helper import assist


class Runner:
    def go(self) -> int:
        return assist()


def run(runner: Runner) -> int:
    return runner.go()
