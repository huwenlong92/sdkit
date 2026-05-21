package password

import "golang.org/x/crypto/bcrypt"

func Hash(plain string) (string, error) {
	return HashCost(plain, bcrypt.DefaultCost)
}

func HashCost(plain string, cost int) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func Verify(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
