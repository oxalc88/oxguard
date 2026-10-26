from src.main import covered, uncovered

def test_covered():
    assert covered() == 1

def test_uncovered():
    assert uncovered() == 2
